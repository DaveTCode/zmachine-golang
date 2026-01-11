package selectstoryui

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davetcode/goz/zmachine"
)

const url = "https://www.ifarchive.org/indexes/if-archive/games/zcode/"

type selectStoryState int

var docStyle = lipgloss.NewStyle().Margin(1, 2)

const (
	loadingStoryList selectStoryState = iota
	choosingStory    selectStoryState = iota
	downloadingStory selectStoryState = iota
)

type story struct {
	name        string
	releaseDate time.Time
	url         string
	description string
	ifdbEntry   string
	ifwiki      string
}

func (s story) Title() string       { return s.name }
func (s story) Description() string { return s.description }
func (s story) FilterValue() string { return s.name + s.description }

type selectStoryModel struct {
	state                  selectStoryState
	storyList              list.Model
	spinner                spinner.Model
	err                    error
	createApplicationModel func(*zmachine.ZMachine, chan<- zmachine.InputResponse, chan<- zmachine.SaveRestoreResponse, <-chan any, []byte, string) tea.Model
	selectedStoryName      string
}

type storiesDownloadedMsg []list.Item
type downloadedStoryMsg []uint8

type errMsg struct{ error }

func (e errMsg) Error() string { return e.error.Error() }

func NewUIModel(createAppModel func(*zmachine.ZMachine, chan<- zmachine.InputResponse, chan<- zmachine.SaveRestoreResponse, <-chan any, []byte, string) tea.Model) tea.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return selectStoryModel{
		state:                  loadingStoryList,
		storyList:              list.New(make([]list.Item, 0), list.NewDefaultDelegate(), 0, 0),
		createApplicationModel: createAppModel,
		spinner:                s,
	}
}

func (m selectStoryModel) Init() tea.Cmd {
	m.storyList.SetShowTitle(false)
	return downloadStoryList
}

func (m selectStoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			s, selected := m.storyList.SelectedItem().(story)
			if selected {
				m.state = downloadingStory
				m.selectedStoryName = s.name

				return m, downloadStory(s)
			}
		}

	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.storyList.SetSize(msg.Width-h, msg.Height-v)

	case storiesDownloadedMsg:
		m.state = choosingStory
		m.storyList.SetShowStatusBar(false)
		m.storyList.SetShowTitle(false)
		return m, m.storyList.SetItems([]list.Item(msg))

	case downloadedStoryMsg:
		zMachineOutputChannel := make(chan any)
		zMachineInputChannel := make(chan zmachine.InputResponse)
		zMachineSaveRestoreChannel := make(chan zmachine.SaveRestoreResponse)
		zMachine := zmachine.LoadRom([]uint8(msg), zMachineInputChannel, zMachineSaveRestoreChannel, zMachineOutputChannel)

		newModel := m.createApplicationModel(zMachine, zMachineInputChannel, zMachineSaveRestoreChannel, zMachineOutputChannel, []byte(msg), m.selectedStoryName)
		return newModel, newModel.Init()

	case errMsg:
		m.err = msg
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.storyList, cmd = m.storyList.Update(msg)
	return m, cmd
}

func (m selectStoryModel) View() string {
	if m.err != nil {
		return docStyle.Render(m.err.Error())
	} else {
		switch m.state {
		case loadingStoryList:
			str := fmt.Sprintf("\n\n   %s Loading stories...\n\n", m.spinner.View())
			return str
		case choosingStory:
			return docStyle.Render(m.storyList.View())
		case downloadingStory:
			str := fmt.Sprintf("\n\n   %s Downloading story...\n\n", m.spinner.View())
			return str
		default:
			return ""
		}
	}
}

func downloadStory(s story) tea.Cmd {
	return func() tea.Msg {
		c := &http.Client{
			Timeout: 60 * time.Second,
		}
		res, err := c.Get(s.url)
		if err != nil {
			return errMsg{err}
		}
		defer res.Body.Close() // nolint:errcheck

		storyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			return errMsg{err}
		}

		return downloadedStoryMsg(storyBytes)
	}
}

func downloadStoryList() tea.Msg {
	c := &http.Client{
		Timeout: 10 * time.Second,
	}
	res, err := c.Get(url)
	if err != nil {
		return errMsg{err}
	}
	defer res.Body.Close() // nolint:errcheck
	if res.StatusCode != 200 {
		return errMsg{}
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return errMsg{err}
	}

	var stories []list.Item

	doc.Find("dl dt").Each(func(i int, s *goquery.Selection) {
		// For each item found, get the title
		title := strings.Replace(s.Find("a").Text(), "◆", "", 1)
		href, _ := s.Find("a").Attr("href")
		match, _ := regexp.Match(".*\\.z[12345678]", []byte(href))

		if match {
			re := regexp.MustCompile(`\d{2}-\w{3}-\d{4}`)
			rawTimeString := s.Find("span").Text()
			releaseDate, _ := time.Parse("02-Jan-2006", re.FindString(rawTimeString))
			var description string
			var ifdbEntry string
			var ifwiki string

			s.NextUntil("dt").Each(func(j int, s2 *goquery.Selection) {
				if strings.Contains(s2.Text(), "IFDB") {
					ifdbEntry, _ = s2.Find("a").Attr("href")
				} else if strings.Contains(s2.Text(), "IFWiki") {
					ifwiki, _ = s2.Find("a").Attr("href")
				} else if len(s2.ChildrenFiltered("p").Nodes) == 1 {
					description = s2.Find("p").Text()
				}
			})

			stories = append(stories, story{
				name:        title,
				releaseDate: releaseDate,
				url:         "https://www.ifarchive.org" + href,
				description: description,
				ifwiki:      ifwiki,
				ifdbEntry:   ifdbEntry,
			})
		}
	})

	return storiesDownloadedMsg(stories)
}
