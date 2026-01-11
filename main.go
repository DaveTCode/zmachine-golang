package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davetcode/goz/selectstoryui"
	"github.com/davetcode/goz/zmachine"
	"github.com/muesli/reflow/wordwrap"
)

var (
	romFilePath  string
	baseAppStyle lipgloss.Style
)

type textUpdateMessage string
type eraseLineRequest zmachine.EraseLineRequest
type eraseWindowRequest zmachine.EraseWindowRequest
type statusBarMessage zmachine.StatusBar
type screenModelMessage zmachine.ScreenModel
type inputRequestMessage zmachine.InputRequest
type saveRequestMessage zmachine.Save
type restoreRequestMessage zmachine.Restore
type restartRequest bool
type runtimeErrorMessage zmachine.RuntimeError
type warningMessage zmachine.Warning
type soundEffectRequest zmachine.SoundEffectRequest

// keyToZChar maps Bubble Tea key messages to Z-machine character codes.
// Function keys are mapped according to the Z-machine spec section 10.5.2.1:
//   - 129-132: Cursor keys (up, down, left, right)
//   - 133-144: Function keys F1-F12
//   - 145-154: Keypad 0-9
//   - 252: Menu click
//   - 253: Mouse double-click
//   - 254: Mouse single-click
func keyToZChar(msg tea.KeyMsg) uint8 {
	switch msg.Type {
	case tea.KeyUp:
		return 129
	case tea.KeyDown:
		return 130
	case tea.KeyLeft:
		return 131
	case tea.KeyRight:
		return 132
	case tea.KeyF1:
		return 133
	case tea.KeyF2:
		return 134
	case tea.KeyF3:
		return 135
	case tea.KeyF4:
		return 136
	case tea.KeyF5:
		return 137
	case tea.KeyF6:
		return 138
	case tea.KeyF7:
		return 139
	case tea.KeyF8:
		return 140
	case tea.KeyF9:
		return 141
	case tea.KeyF10:
		return 142
	case tea.KeyF11:
		return 143
	case tea.KeyF12:
		return 144
	case tea.KeyEscape:
		return 27 // ESC character
	case tea.KeyEnter:
		return 13 // Carriage return
	case tea.KeyDelete:
		return 8 // Delete/backspace
	default:
		return 0
	}
}

// isValidTerminator checks if a key code is in the list of valid terminators
func isValidTerminator(keyCode uint8, validTerminators []uint8) bool {
	if keyCode == 0 {
		return false
	}
	for _, t := range validTerminators {
		if t == keyCode {
			return true
		}
	}
	return false
}

type runningStoryState int

const (
	appRunning             runningStoryState = iota
	appWaitingForInput     runningStoryState = iota
	appWaitingForCharacter runningStoryState = iota
)

type runStoryModel struct {
	outputChannel            <-chan any
	sendChannel              chan<- zmachine.InputResponse
	saveRestoreChannel       chan<- zmachine.SaveRestoreResponse
	zMachine                 *zmachine.ZMachine
	romBytes                 []byte
	romFilePath              string
	statusBar                zmachine.StatusBar
	screenModel              zmachine.ScreenModel
	lowerWindowTextPreStyled string
	lowerWindowText          string
	upperWindowText          []string
	upperWindowStyle         [][]lipgloss.Style
	appState                 runningStoryState
	validTerminators         []uint8 // Valid terminating characters for current input
	inputBox                 textinput.Model
	width                    int
	height                   int
	backgroundStyle          lipgloss.Style
	statusBarStyle           lipgloss.Style
	upperWindowStyleCurrent  lipgloss.Style
	lowerWindowStyle         lipgloss.Style
	runtimeError             string
}

func (m runStoryModel) Init() tea.Cmd {
	return tea.Batch(
		waitForInterpreter(m.outputChannel),
		runInterpreter(m.zMachine),
		tea.Sequence(
			tea.SetWindowTitle(romFilePath),
			tea.WindowSize(),
		),
	)
}

func runInterpreter(z *zmachine.ZMachine) tea.Cmd {
	return func() tea.Msg {
		z.Run()

		return nil
	}
}

func (m runStoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg: // Handle window resize events
		m.width = msg.Width
		m.height = msg.Height

		if m.height < len(m.upperWindowText) {
			m.upperWindowText = m.upperWindowText[:m.height]
			m.upperWindowStyle = m.upperWindowStyle[:m.height]
		} else {
			for range int(math.Min(float64(m.height-len(m.upperWindowText)), float64(m.screenModel.UpperWindowHeight))) {
				m.upperWindowText = append(m.upperWindowText, strings.Repeat(" ", m.width))
				m.upperWindowStyle = append(m.upperWindowStyle, slices.Repeat([]lipgloss.Style{baseAppStyle}, m.width))
			}
		}

		// Keep the upper window at exactly the size of the screen
		for ix, row := range m.upperWindowText {
			if m.width < len(row) {
				m.upperWindowText[ix] = row[:m.width]
				m.upperWindowStyle[ix] = m.upperWindowStyle[ix][:m.width]
			} else if m.width > len(row) {
				for ii := len(row); ii < m.width; ii++ {
					m.upperWindowText[ix] = m.upperWindowText[ix] + " "
					m.upperWindowStyle[ix] = append(m.upperWindowStyle[ix], baseAppStyle)
				}
			}
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			os.Exit(0)
		}

		switch m.appState {
		case appWaitingForCharacter:
			m.appState = appRunning
			if len(msg.Runes) > 0 {
				m.sendChannel <- zmachine.InputResponse{Text: string(msg.Runes[0]), TerminatingKey: 0}
			} else {
				// Map special keys to Z-machine character codes
				keyCode := keyToZChar(msg)
				m.sendChannel <- zmachine.InputResponse{Text: "", TerminatingKey: keyCode}
			}
		case appWaitingForInput:
			// Check if this key is a valid terminator
			keyCode := keyToZChar(msg)
			if msg.Type == tea.KeyEnter || isValidTerminator(keyCode, m.validTerminators) {
				m.appState = appRunning
				m.lowerWindowText += m.inputBox.Value() + "\n"
				terminatingKey := uint8(13) // Default to carriage return
				if msg.Type != tea.KeyEnter {
					terminatingKey = keyCode
				}
				m.sendChannel <- zmachine.InputResponse{Text: m.inputBox.Value(), TerminatingKey: terminatingKey}
				m.inputBox.SetValue("")
			}
		}

	case textUpdateMessage:
		if m.screenModel.LowerWindowActive {
			// In anything other than v6 the bottom window is append only (I think - TODO)
			m.lowerWindowText += string(msg)
		} else {
			// Upper window - handle text, splitting on newlines
			text := string(msg)
			segments := strings.Split(text, "\n")
			cursorX := m.screenModel.UpperWindowCursorX
			cursorY := m.screenModel.UpperWindowCursorY

			for segIdx, segment := range segments {
				if cursorY >= 0 && cursorY < len(m.upperWindowText) {
					row := m.upperWindowText[cursorY]

					// Update styles for each character being written
					if cursorY < len(m.upperWindowStyle) {
						for i := 0; i < len(segment) && cursorX+i < len(m.upperWindowStyle[cursorY]); i++ {
							m.upperWindowStyle[cursorY][cursorX+i] = m.upperWindowStyleCurrent
						}
					}

					if cursorX < len(row) {
						before := row[:cursorX]
						// Replace characters at cursor position (not insert)
						afterStart := cursorX + len(segment)
						after := ""
						if afterStart < len(row) {
							after = row[afterStart:]
						}
						fullText := before + segment + after
						if len(fullText) > m.width {
							fullText = fullText[:m.width]
						}
						m.upperWindowText[cursorY] = fullText
					}
				}

				// After each segment (except the last), move to next line
				if segIdx < len(segments)-1 {
					cursorY++
					cursorX = 0
				}
			}
		}

		return m, waitForInterpreter(m.outputChannel)

	case inputRequestMessage:
		m.appState = appWaitingForInput
		m.validTerminators = msg.ValidTerminators
		return m, waitForInterpreter(m.outputChannel)

	case saveRequestMessage:
		if msg.NumBytes == 0 {
			saveData := m.zMachine.ExportSaveState()
			filename := msg.Filename
			if filename == "" {
				filename = m.defaultSaveFilename()
			}
			// TODO: If msg.Prompt is true, ask user for filename
			err := os.WriteFile(filename, saveData, 0644)
			if err != nil {
				m.saveRestoreChannel <- zmachine.SaveResponse{Success: false, Result: 0}
			} else {
				m.saveRestoreChannel <- zmachine.SaveResponse{Success: true, Result: 1}
			}
		} else {
			// TODO: Implement auxiliary save
			m.saveRestoreChannel <- zmachine.SaveResponse{Success: false, Result: 0}
		}
		return m, waitForInterpreter(m.outputChannel)

	case restoreRequestMessage:
		if msg.NumBytes == 0 {
			filename := msg.Filename
			if filename == "" {
				filename = m.defaultSaveFilename()
			}
			// TODO: If msg.Prompt is true, ask user for filename
			data, err := os.ReadFile(filename)
			if err != nil {
				m.saveRestoreChannel <- zmachine.RestoreResponse{Success: false, Result: 0}
			} else {
				m.saveRestoreChannel <- zmachine.RestoreResponse{Success: true, Result: 2, Data: data}
			}
		} else {
			// TODO: Implement auxiliary restore
			m.saveRestoreChannel <- zmachine.RestoreResponse{Success: false, Result: 0}
		}
		return m, waitForInterpreter(m.outputChannel)

	case zmachine.StateChangeRequest:
		switch msg {
		case zmachine.WaitForCharacter:
			m.appState = appWaitingForCharacter
		case zmachine.Running:
			m.appState = appRunning
		}
		return m, waitForInterpreter(m.outputChannel)

	case statusBarMessage:
		m.statusBar = zmachine.StatusBar(msg)
		return m, waitForInterpreter(m.outputChannel)

	case screenModelMessage:
		m.screenModel = zmachine.ScreenModel(msg)
		if len(m.upperWindowText) != m.screenModel.UpperWindowHeight {
			if m.zMachine.Core.Version == 3 {
				for row := range m.screenModel.UpperWindowHeight {
					m.upperWindowText[row] = strings.Repeat(" ", m.width)
					m.upperWindowStyle[row] = slices.Repeat([]lipgloss.Style{baseAppStyle}, m.width)
				}
			} else if len(m.upperWindowText) > m.screenModel.UpperWindowHeight {
				m.upperWindowText = m.upperWindowText[:m.screenModel.UpperWindowHeight]
				m.upperWindowStyle = m.upperWindowStyle[:m.screenModel.UpperWindowHeight]
			} else {
				for range m.screenModel.UpperWindowHeight - len(m.upperWindowText) {
					m.upperWindowText = append(m.upperWindowText, strings.Repeat(" ", m.width))
					m.upperWindowStyle = append(m.upperWindowStyle, slices.Repeat([]lipgloss.Style{baseAppStyle}, m.width))
				}
			}
		}

		// Only flush the lower window text to the prestyled buffer when there's a change to the screen
		// model to avoid performance hit by too many ascii codes
		prerenderLowerWindowText(&m)

		m.lowerWindowStyle = m.lowerWindowStyle.
			Background(lipgloss.Color(m.screenModel.LowerWindowBackground.ToHex())).
			Foreground(lipgloss.Color(m.screenModel.LowerWindowForeground.ToHex())).
			Bold(m.screenModel.LowerWindowTextStyle&zmachine.Bold == zmachine.Bold).
			Italic(m.screenModel.LowerWindowTextStyle&zmachine.Italic == zmachine.Italic).
			Reverse(m.screenModel.LowerWindowTextStyle&zmachine.ReverseVideo == zmachine.ReverseVideo).
			Inline(true)
		m.upperWindowStyleCurrent = m.upperWindowStyleCurrent.
			Background(lipgloss.Color(m.screenModel.UpperWindowBackground.ToHex())).
			Foreground(lipgloss.Color(m.screenModel.UpperWindowForeground.ToHex())).
			Bold(m.screenModel.UpperWindowTextStyle&zmachine.Bold == zmachine.Bold).
			Italic(m.screenModel.UpperWindowTextStyle&zmachine.Italic == zmachine.Italic).
			Reverse(m.screenModel.UpperWindowTextStyle&zmachine.ReverseVideo == zmachine.ReverseVideo)
		m.statusBarStyle = m.lowerWindowStyle.Reverse(true)
		m.backgroundStyle = m.backgroundStyle.
			Background(lipgloss.Color(m.screenModel.DefaultLowerWindowBackground.ToHex())).
			Foreground(lipgloss.Color(m.screenModel.DefaultLowerWindowForeground.ToHex()))

		return m, waitForInterpreter(m.outputChannel)

	case restartRequest:
		// Reload the ROM from stored bytes
		zMachineOutputChannel := make(chan any)
		zMachineInputChannel := make(chan zmachine.InputResponse)
		zMachineSaveRestoreChannel := make(chan zmachine.SaveRestoreResponse)
		m.zMachine = zmachine.LoadRom(m.romBytes, zMachineInputChannel, zMachineSaveRestoreChannel, zMachineOutputChannel)
		m.outputChannel = zMachineOutputChannel
		m.sendChannel = zMachineInputChannel
		m.saveRestoreChannel = zMachineSaveRestoreChannel

		// Clear screen state
		m.lowerWindowText = ""
		m.lowerWindowTextPreStyled = ""
		for row := range len(m.upperWindowText) {
			m.upperWindowText[row] = strings.Repeat(" ", m.width)
			m.upperWindowStyle[row] = slices.Repeat([]lipgloss.Style{baseAppStyle}, m.width)
		}
		m.appState = appRunning
		return m, tea.Batch(
			waitForInterpreter(m.outputChannel),
			runInterpreter(m.zMachine),
		)

	case eraseLineRequest:
		// Don't think you can erase line in lower window
		if !m.screenModel.LowerWindowActive {
			line := m.screenModel.UpperWindowCursorY
			start := m.screenModel.UpperWindowCursorX
			if line >= 0 && line < len(m.upperWindowText) && start >= 0 && start < len(m.upperWindowText[line]) {
				row := m.upperWindowText[line]
				before := row[:start]
				after := ""
				if start < len(row) {
					after = row[start:]
				}
				fullText := before + strings.Repeat(" ", len(after))
				if len(fullText) > m.width {
					fullText = fullText[:m.width]
				}
				m.upperWindowText[line] = fullText
			}
		}

	case eraseWindowRequest:
		switch int(msg) {
		case -2: // Keep split windows and clear both
			m.lowerWindowText = ""
			m.lowerWindowTextPreStyled = ""
			for row := range m.screenModel.UpperWindowHeight {
				m.upperWindowText[row] = strings.Repeat(" ", m.width)
				m.upperWindowStyle[row] = slices.Repeat([]lipgloss.Style{baseAppStyle}, m.width)
			}
		case -1: // Unsplit the window and clear both
			m.lowerWindowText = ""
			m.lowerWindowTextPreStyled = ""
			for row := range len(m.upperWindowText) {
				m.upperWindowText[row] = strings.Repeat(" ", m.width)
				m.upperWindowStyle[row] = slices.Repeat([]lipgloss.Style{baseAppStyle}, m.width)
			}
		case 0: // Clear lower window
			m.lowerWindowText = ""
			m.lowerWindowTextPreStyled = ""
		case 1: // Clear upper window
			for row := range m.screenModel.UpperWindowHeight {
				m.upperWindowText[row] = strings.Repeat(" ", m.width)
				m.upperWindowStyle[row] = slices.Repeat([]lipgloss.Style{baseAppStyle}, m.width)
			}
		default:
			m.runtimeError = fmt.Sprintf("Unexpected erase_window value: %d", int(msg))
			return m, tea.Quit
		}

		return m, waitForInterpreter(m.outputChannel)

	case runtimeErrorMessage:
		m.runtimeError = string(msg)
		return m, tea.Quit

	case warningMessage:
		// Warnings are non-fatal - print to stderr and continue
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		return m, waitForInterpreter(m.outputChannel)

	case soundEffectRequest:
		switch msg.SoundNumber {
		case 1: // High pitched beep, no repeats, volume etc
			fmt.Print("\a")
		case 2: // Low pitched beep, no repeats, volume etc
			fmt.Print("\a")
		default:
			// Not supporting other sound effects at the moment
			if msg.Routine != 0 {
				// Warnings are non-fatal - print to stderr and continue
				fmt.Fprintf(os.Stderr, "Warning: sound effect (%d) expecting routine call after completion not supported\n", msg.Effect)
			}
		}

		return m, waitForInterpreter(m.outputChannel)
	}

	if m.appState == appWaitingForInput {
		m.inputBox, cmd = m.inputBox.Update(msg)
	}

	return m, cmd
}

func prerenderLowerWindowText(m *runStoryModel) {
	if m.lowerWindowText != "" {
		lines := strings.Split(m.lowerWindowText, "\n")
		for ix, line := range lines {
			lines[ix] = m.lowerWindowStyle.Render(line)
		}
		m.lowerWindowTextPreStyled += strings.Join(lines, "\n")
		m.lowerWindowText = ""
	}
}

// defaultSaveFilename derives a save filename from the ROM file path.
// It replaces the .z* extension with .sav, e.g., "zork1.z1" -> "zork1.sav"
func (m runStoryModel) defaultSaveFilename() string {
	if m.romFilePath == "" {
		return "game.sav"
	}
	base := filepath.Base(m.romFilePath)
	// Remove .z* extension (z1, z2, z3, z4, z5, z6, z7, z8)
	ext := filepath.Ext(base)
	if len(ext) >= 2 && (ext[1] == 'z' || ext[1] == 'Z') {
		base = base[:len(base)-len(ext)]
	}
	return base + ".sav"
}

func createStatusLine(width int, placeName string, scoreOrHours int, movesOrMinutes int, isTimeBasedGame bool) string {
	rightHandSide := fmt.Sprintf("Score: %d    Moves %d", scoreOrHours, movesOrMinutes)

	if isTimeBasedGame {
		rightHandSide = fmt.Sprintf("Time: %d:%d", scoreOrHours, movesOrMinutes)
	}

	// Too narrow to show properly so just show as much of the score/time/moves as we can manage
	if len(rightHandSide) >= width {
		return rightHandSide[:width]
	}

	if len(placeName)+len(rightHandSide)+1 >= width {
		return fmt.Sprintf("%s %s", placeName[:width-len(rightHandSide)-1], rightHandSide)
	}

	numberSpaces := width - len(placeName) - len(rightHandSide)

	return fmt.Sprintf("%s%s%s", placeName, strings.Repeat(" ", numberSpaces), rightHandSide)
}

func (m runStoryModel) View() string {
	// If there was a runtime error, display it
	if m.runtimeError != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true)
		return fmt.Sprintf("\n%s\n\n%s\n", errorStyle.Render("Z-Machine Error:"), m.runtimeError)
	}

	// Wait until the screen has loaded properly to print anything
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	s := strings.Builder{}
	lowerWindowHeight := m.height

	if m.statusBar.PlaceName != "" {
		s.WriteString(m.statusBarStyle.Render(createStatusLine(m.width, m.statusBar.PlaceName, m.statusBar.Score, m.statusBar.Moves, m.statusBar.IsTimeBased)))
		s.WriteString(m.lowerWindowStyle.Render("\n"))
		lowerWindowHeight -= 2 // 2 fewer lines to work with if there's a status bar
	} else {
		lowerWindowHeight -= m.screenModel.UpperWindowHeight

		var text strings.Builder
		var currentText strings.Builder
		var currentStyle lipgloss.Style
		for row, styleRow := range m.upperWindowStyle {
			for col, chrStyle := range styleRow {
				if chrStyle.GetBackground() != currentStyle.GetBackground() ||
					chrStyle.GetForeground() != currentStyle.GetForeground() ||
					chrStyle.GetBlink() != currentStyle.GetBlink() ||
					chrStyle.GetBold() != currentStyle.GetBold() ||
					chrStyle.GetItalic() != currentStyle.GetItalic() ||
					chrStyle.GetReverse() != currentStyle.GetReverse() {
					if currentText.Len() > 0 {
						text.WriteString(currentStyle.Render(currentText.String()))
					}
					currentStyle = chrStyle
					currentText.Reset()
				}
				currentText.WriteRune([]rune(m.upperWindowText[row])[col])
			}
			currentText.WriteByte('\n')
		}
		if currentText.Len() > 0 {
			text.WriteString(currentStyle.Render(currentText.String()))
		}
		s.WriteString(text.String())
	}

	// Text must be pre-rendered in relevant style in the outputText as styles
	// can change during screen usage
	prerenderLowerWindowText(&m)
	fullLowerWindowText := m.lowerWindowTextPreStyled

	wordWrappedBody := wordwrap.String(fullLowerWindowText, m.width)

	lines := strings.Split(wordWrappedBody, "\n")

	if len(lines) > lowerWindowHeight-2 {
		lines = lines[len(lines)-lowerWindowHeight+2:]
	}
	s.WriteString(strings.Join(lines, "\n"))

	if m.appState == appWaitingForInput {
		// TODO - Can we use a nicer style for this somehow?
		s.WriteString(m.lowerWindowStyle.Render("\n" + m.inputBox.View()))
	}

	return m.backgroundStyle.
		Width(m.width).
		Height(m.height).
		Render(s.String())
}

func waitForInterpreter(sub <-chan any) tea.Cmd {
	return func() tea.Msg {
		msg := <-sub
		switch msg := msg.(type) {
		case zmachine.InputRequest:
			return inputRequestMessage(msg)
		case zmachine.Save:
			return saveRequestMessage(msg)
		case zmachine.Restore:
			return restoreRequestMessage(msg)
		case zmachine.StateChangeRequest:
			return msg // Pass through directly, handled in Update
		case zmachine.EraseWindowRequest:
			return eraseWindowRequest(msg)
		case zmachine.StatusBar:
			return statusBarMessage(msg)
		case zmachine.ScreenModel:
			return screenModelMessage(msg)
		case string:
			return textUpdateMessage(msg)
		case zmachine.Quit:
			return tea.Quit()
		case zmachine.Restart:
			return restartRequest(true)
		case zmachine.RuntimeError:
			return runtimeErrorMessage(msg)
		case zmachine.Warning:
			return warningMessage(msg)
		default:
			return runtimeErrorMessage(zmachine.RuntimeError("Invalid message type sent from interpreter"))
		}
	}
}

func init() {
	flag.StringVar(&romFilePath, "rom", "", "The path of a z-machine rom")
	flag.Parse()
}

func newApplicationModel(zMachine *zmachine.ZMachine, inputChannel chan<- zmachine.InputResponse, saveRestoreChannel chan<- zmachine.SaveRestoreResponse, outputChannel <-chan any, romBytes []byte, romPath string) tea.Model {

	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20
	ti.Prompt = ""

	return runStoryModel{
		outputChannel:           outputChannel,
		sendChannel:             inputChannel,
		saveRestoreChannel:      saveRestoreChannel,
		zMachine:                zMachine,
		romBytes:                romBytes,
		romFilePath:             romPath,
		appState:                appRunning,
		validTerminators:        []uint8{13}, // Default to just Enter
		inputBox:                ti,
		upperWindowStyleCurrent: lipgloss.NewStyle(),
		lowerWindowStyle:        lipgloss.NewStyle(),
		statusBarStyle:          lipgloss.NewStyle(),
		backgroundStyle:         lipgloss.NewStyle(),
	}
}

func main() {
	var model tea.Model

	if romFilePath != "" {
		romFileBytes, err := os.ReadFile(romFilePath)
		if err != nil {
			panic(err)
		}
		zMachineOutputChannel := make(chan any)
		zMachineInputChannel := make(chan zmachine.InputResponse)
		zMachineSaveRestoreChannel := make(chan zmachine.SaveRestoreResponse)
		zMachine := zmachine.LoadRom(romFileBytes, zMachineInputChannel, zMachineSaveRestoreChannel, zMachineOutputChannel)

		model = newApplicationModel(zMachine, zMachineInputChannel, zMachineSaveRestoreChannel, zMachineOutputChannel, romFileBytes, romFilePath)
	} else {
		model = selectstoryui.NewUIModel(newApplicationModel)
	}

	tui := tea.NewProgram(model) //, tea.WithAltScreen())

	if _, err := tui.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
