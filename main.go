package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davetcode/goz/zmachine"
	"github.com/muesli/reflow/wordwrap"
)

var (
	romFilePath  string
	baseAppStyle lipgloss.Style
)

type textUpdateMessage string
type stateUpdateMessage zmachine.StateChangeRequest
type StatusBarMessage zmachine.StatusBar
type ScreenModelMessage zmachine.ScreenModel

type appState int

const (
	appRunning         appState = iota
	appWaitingForInput appState = iota
)

type applicationModel struct {
	outputChannel            <-chan interface{}
	sendChannel              chan<- string
	zMachine                 *zmachine.ZMachine
	statusBar                zmachine.StatusBar
	screenModel              zmachine.ScreenModel
	lowerWindowTextPreStyled string
	lowerWindowText          string
	upperWindowText          []string
	upperWindowStyle         [][]lipgloss.Style
	appState                 appState
	inputBox                 textinput.Model
	width                    int
	height                   int
	backgroundStyle          lipgloss.Style
	statusBarStyle           lipgloss.Style
	upperWindowStyleCurrent  lipgloss.Style
	lowerWindowStyle         lipgloss.Style
}

func (m applicationModel) Init() tea.Cmd {
	return tea.Batch(
		waitForInterpreter(m.outputChannel),
		runInterpreter(m.zMachine),
		tea.SetWindowTitle(romFilePath),
	)
}

func runInterpreter(z *zmachine.ZMachine) tea.Cmd {
	return func() tea.Msg {
		z.Run()

		return nil
	}
}

func (m applicationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

		switch msg.Type {
		case tea.KeyEnter: // TODO - Some versions have different keys which trigger this
			m.appState = appRunning
			m.lowerWindowText += m.inputBox.Value() + "\n"
			m.sendChannel <- m.inputBox.Value()
			m.inputBox.SetValue("")
			return m, nil
		}

	case textUpdateMessage:
		if m.screenModel.LowerWindowActive {
			// In anything other than v6 the bottom window is append only (I think - TODO)
			m.lowerWindowText += string(msg)
		} else {
			if m.screenModel.UpperWindowCursorY > 0 && m.screenModel.UpperWindowCursorY < len(m.upperWindowText) {
				row := m.upperWindowText[m.screenModel.UpperWindowCursorY]
				text := string(msg)

				// Need to track what style each rune is written in so we can track cursor position based on runes but still
				// render using the original style they were written with. Rendering first will fill the text with ansi chars
				// for specifying the colors/styles
				for i := m.screenModel.UpperWindowCursorX; i < int(math.Min(float64(len(row)), float64(len(text)))); i++ {
					m.upperWindowStyle[m.screenModel.UpperWindowCursorY][i] = m.upperWindowStyleCurrent
				}

				if m.screenModel.UpperWindowCursorX < len(row) {
					before := row[:m.screenModel.UpperWindowCursorX]
					after := row[m.screenModel.UpperWindowCursorX:]
					fullText := before + text + after
					m.upperWindowText[m.screenModel.UpperWindowCursorY] = fullText[:m.width] // Truncate the text to the width of the screen
				} else {
					// TODO Nothing happens here maybe? Trying to print on a column outside the screen
				}
			} else {
				// TODO - Nothing happens here, trying to print on a row that can't be shown
			}
		}

		return m, waitForInterpreter(m.outputChannel)

	case stateUpdateMessage:
		switch zmachine.StateChangeRequest(msg) {
		case zmachine.WaitForInput:
			m.appState = appWaitingForInput
		case zmachine.Running:
			m.appState = appRunning
		}
		return m, waitForInterpreter(m.outputChannel)

	case StatusBarMessage:
		m.statusBar = zmachine.StatusBar(msg)
		return m, waitForInterpreter(m.outputChannel)

	case ScreenModelMessage:
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

		m.lowerWindowStyle = m.lowerWindowStyle.
			Background(lipgloss.Color(m.screenModel.LowerWindowBackground.ToHex())).
			Foreground(lipgloss.Color(m.screenModel.LowerWindowForeground.ToHex())).
			Bold(m.screenModel.LowerWindowTextStyle&zmachine.Bold == zmachine.Bold).
			Italic(m.screenModel.LowerWindowTextStyle&zmachine.Italic == zmachine.Italic).
			Reverse(m.screenModel.LowerWindowTextStyle&zmachine.ReverseVideo == zmachine.ReverseVideo)
		m.upperWindowStyleCurrent = m.upperWindowStyleCurrent.
			Background(lipgloss.Color(m.screenModel.UpperWindowBackground.ToHex())).
			Foreground(lipgloss.Color(m.screenModel.UpperWindowForeground.ToHex())).
			Bold(m.screenModel.UpperWindowTextStyle&zmachine.Bold == zmachine.Bold).
			Italic(m.screenModel.UpperWindowTextStyle&zmachine.Italic == zmachine.Italic).
			Reverse(m.screenModel.UpperWindowTextStyle&zmachine.ReverseVideo == zmachine.ReverseVideo)
		m.statusBarStyle = m.lowerWindowStyle.Reverse(true)
		m.backgroundStyle = m.backgroundStyle.
			Background(lipgloss.Color(m.screenModel.LowerWindowBackground.ToHex())).
			Foreground(lipgloss.Color(m.screenModel.LowerWindowForeground.ToHex()))

		// Only flush the lower window text to the prestyled buffer when there's a change to the screen
		// model to avoid performance hit by too many ascii codes
		if m.lowerWindowText != "" {
			m.lowerWindowTextPreStyled += m.lowerWindowStyle.Render(m.lowerWindowText)
			m.lowerWindowText = ""
		}

		return m, waitForInterpreter(m.outputChannel)
	}

	if m.appState == appWaitingForInput {
		m.inputBox, cmd = m.inputBox.Update(msg)
	}

	return m, cmd
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

func (m applicationModel) View() string {
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
		for row, styleRow := range m.upperWindowStyle {
			for col, chrStyle := range styleRow {
				chr := string([]rune(m.upperWindowText[row])[col])
				text.WriteString(chrStyle.Render(chr))
			}
			text.WriteByte('\n')
		}
		s.WriteString(text.String())
	}

	// Text must be pre-rendered in relevant style in the outputText as styles
	// can change during screen usage
	var fullLowerWindowText string
	if m.lowerWindowText != "" && m.lowerWindowText[len(m.lowerWindowText)-1] == '>' {
		fullLowerWindowText = m.lowerWindowTextPreStyled + m.lowerWindowStyle.Render(m.lowerWindowText[:len(m.lowerWindowText)-1])
	} else {
		fullLowerWindowText = m.lowerWindowTextPreStyled + m.lowerWindowStyle.Render(m.lowerWindowText)
	}

	wordWrappedBody := wordwrap.String(fullLowerWindowText, m.width)

	lines := strings.Split(wordWrappedBody, "\n")

	if len(lines) > lowerWindowHeight-2 {
		lines = lines[len(lines)-lowerWindowHeight+2:]
	}
	s.WriteString(strings.Join(lines, "\n"))

	if m.appState == appWaitingForInput {
		// TODO - Can we use a nicer style for this somehow?
		s.WriteString(m.lowerWindowStyle.Inline(true).Render("> " + m.inputBox.View()))
	}

	return m.backgroundStyle.
		Width(m.width).
		Height(m.height).
		Render(s.String())
}

func waitForInterpreter(sub <-chan interface{}) tea.Cmd {
	return func() tea.Msg {
		msg := <-sub
		switch msg := msg.(type) {
		case zmachine.StateChangeRequest:
			return stateUpdateMessage(msg)
		case zmachine.StatusBar:
			return StatusBarMessage(msg)
		case zmachine.ScreenModel:
			return ScreenModelMessage(msg)
		case string:
			return textUpdateMessage(msg)
		case zmachine.Quit:
			return tea.Quit()
		default:
			panic("Invalid message type sent from interpreter")
		}
	}
}

func init() {
	flag.StringVar(&romFilePath, "rom", "zork1.z1", "The path of a z-machine rom")
	flag.Parse()
}

func newApplicationModel(zMachine *zmachine.ZMachine, inputChannel chan<- string, outputChannel <-chan interface{}) applicationModel {

	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20
	ti.Prompt = ""

	return applicationModel{
		outputChannel:           outputChannel,
		sendChannel:             inputChannel,
		zMachine:                zMachine,
		appState:                appRunning,
		inputBox:                ti,
		upperWindowStyleCurrent: lipgloss.NewStyle(),
		lowerWindowStyle:        lipgloss.NewStyle(),
		statusBarStyle:          lipgloss.NewStyle(),
		backgroundStyle:         lipgloss.NewStyle(),
	}
}

func main() {
	zMachineOutputChannel := make(chan interface{})
	zMachineInputChannel := make(chan string)

	romFileBytes, err := os.ReadFile(romFilePath)
	if err != nil {
		panic(err)
	}
	zMachine := zmachine.LoadRom(romFileBytes, zMachineInputChannel, zMachineOutputChannel)

	appModel := newApplicationModel(zMachine, zMachineInputChannel, zMachineOutputChannel)

	tui := tea.NewProgram(appModel, tea.WithAltScreen())

	if _, err := tui.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
