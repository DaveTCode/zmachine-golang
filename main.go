package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const speedOfTick = 50 * time.Millisecond

var (
	romFilePath string

	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)

	statusMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#04B575"}).
				Render
)

type tickMsg time.Time

type applicationModel struct {
	zMachine    *ZMachine
	disassembly []string
}

func newApplicationModel(filePath string) applicationModel {
	romFileBytes, err := os.ReadFile(filePath)
	if err != nil {
		panic(err)
	}

	return applicationModel{
		zMachine: LoadRom(romFileBytes),
	}
}

func (m applicationModel) Init() tea.Cmd {
	return tea.Batch(tea.SetWindowTitle(romFilePath), tick())
}

func (m applicationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	case tickMsg:
		m.disassembly = append(m.disassembly, fmt.Sprintf("0x%x: 0x%x", m.zMachine.callStack.peek().pc, m.zMachine.memory[m.zMachine.callStack.peek().pc]))
		m.zMachine.StepMachine()
		return m, tick()
	}

	return m, nil
}

func (m applicationModel) View() string {
	s := strings.Builder{}

	s.WriteString(appStyle.Render(m.zMachine.text))

	return s.String()
}

func tick() tea.Cmd {
	return tea.Tick(speedOfTick, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func init() {
	flag.StringVar(&romFilePath, "rom", "zork1.z1", "The path of a z-machine rom")
	flag.Parse()
}

func main() {
	applicationModel := newApplicationModel(romFilePath)
	tui := tea.NewProgram(applicationModel)

	if _, err := tui.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
