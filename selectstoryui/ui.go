package selectstoryui

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davetcode/goz/zmachine"
)

const url = "https://www.ifarchive.org/indexes/if-archive/games/zcode/"

type selectStoryState int

var docStyle = lipgloss.NewStyle().Margin(1, 2)

const (
	LoadingStoryList selectStoryState = iota
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

type SelectStoryModel struct {
	State                  selectStoryState
	StoryList              list.Model
	err                    error
	CreateApplicationModel func(*zmachine.ZMachine, chan<- string, <-chan interface{}) tea.Model
}

type storiesDownloadedMsg []list.Item
type downloadedStoryMsg []uint8

type errMsg struct{ error }

func (e errMsg) Error() string { return e.error.Error() }

func (m SelectStoryModel) Init() tea.Cmd {
	m.StoryList.Title = "Z-Machine stories from ifarchive"
	return downloadStoryList
}

func (m SelectStoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			s, selected := m.StoryList.SelectedItem().(story)
			if selected {
				m.State = downloadingStory

				return m, downloadStory(s)
			}
		}

	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.StoryList.SetSize(msg.Width-h, msg.Height-v)

	case storiesDownloadedMsg:
		return m, m.StoryList.SetItems([]list.Item(msg))

	case downloadedStoryMsg:
		zMachineOutputChannel := make(chan interface{})
		zMachineInputChannel := make(chan string)
		zMachine := zmachine.LoadRom([]uint8(msg), zMachineInputChannel, zMachineOutputChannel)

		newModel := m.CreateApplicationModel(zMachine, zMachineInputChannel, zMachineOutputChannel)
		return newModel, newModel.Init()

	case errMsg:
		m.err = msg
		return m, nil
	}

	var cmd tea.Cmd
	m.StoryList, cmd = m.StoryList.Update(msg)
	return m, cmd
}

func (m SelectStoryModel) View() string {
	if m.err != nil {
		return docStyle.Render(m.err.Error())
	}
	return docStyle.Render(m.StoryList.View())
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
		title := s.Find("a").Text()
		href, _ := s.Find("a").Attr("href")
		match, _ := regexp.Match(".*\\.z[12345678]", []byte(href))

		if match {
			re := regexp.MustCompile(`\d{2}-\w{3}-\d{4}`)
			rawTimeString := s.Find("span").Text()
			releaseDate, _ := time.Parse("01-Jan-1980", re.FindString(rawTimeString))
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
