package main

import (
	"fmt"
	"log"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/evertras/bubble-table/table"
)

const (
	viewSearch    = "search"
	viewTorrents  = "torrents"
	viewDownloads = "downloads"
	viewLibrary   = "library"
)

type downloadCreateMsg struct{}

type Styles struct {
	BorderColor lipgloss.Color
	InputField  lipgloss.Style
	SelectedRow lipgloss.Style
}

func DefaultStyles() *Styles {
	return &Styles{
		BorderColor: lipgloss.Color("#5e81ac"),
		InputField: lipgloss.NewStyle().
			BorderForeground(lipgloss.Color("#81a1c1")).
			BorderStyle(lipgloss.ThickBorder()).
			Width(50).
			Padding(0, 1),
		SelectedRow: lipgloss.NewStyle().
			Background(lipgloss.Color("#88c0d0")).
			Foreground(lipgloss.Color("#2e3440")).
			Bold(true),
	}
}

type model struct {
	torrents       []Torrent
	Downloading    []Torrent
	Library        []Torrent
	searchField    textinput.Model
	search         string
	styles         *Styles
	view           string
	err            error
	torrentTable   table.Model
	currentPage    int
	rowsPerPage    int
	selectedID     int
	width          int
	height         int
	downloadStatus bool
	ticker         *time.Ticker
}

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
		return struct{}{}
	})
}

func New() *model {
	searchField := textinput.New()
	searchField.Placeholder = "Sail the seas"
	searchField.Focus()

	m := &model{
		styles:         DefaultStyles(),
		searchField:    searchField,
		view:           viewSearch,
		rowsPerPage:    1,
		downloadStatus: false,
		currentPage:    0,
		selectedID:     0,
	}

	return m
}

func (m *model) CreateTorrentRows(torrents []Torrent) []table.Row {
	rows := make([]table.Row, len(torrents))
	for i, torrent := range torrents {
		rows[i] = table.NewRow(table.RowData{
			"name":      torrent.Name,
			"size":      torrent.Size,
			"seeders":   torrent.Seeders,
			"leechers":  torrent.Leechers,
			"num_files": torrent.NumFiles,
		})
	}
	return rows
}

func InitTable(columns []table.Column, rows []table.Row) table.Model {
	return table.New(columns).WithRows(rows).SortByDesc("seeders")
}

func CreateTorrentColumns(width int) []table.Column {
	nameWidth := width / 2
	return []table.Column{
		table.NewColumn("name", "Name", nameWidth),
		table.NewColumn("size", "Size", 9),
		table.NewColumn("seeders", "Seeders", 8),
		table.NewColumn("leechers", "Leechers", 8),
		table.NewColumn("num_files", "Files", 6),
	}
}

func (m *model) UpdateTables() {
	m.torrentTable = InitTable(CreateTorrentColumns(m.width), m.CreateTorrentRows(m.torrents))
}

func (m *model) Init() tea.Cmd {
	err := m.loadDownloadState()
	if err != nil {
		log.Fatalf("Error loading Download data: %v", err)
	}
	go m.GetDownloadInfo()

	return tea.Batch(
		tick(),
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		m.UpdateTables()
	case downloadCreateMsg:
		m.view = viewDownloads
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.saveDownloadState()
			return m, tea.Quit
		case "ctrl+d":
			m.searchField.Blur()
			m.view = viewDownloads
			m.currentPage, m.selectedID = 0, 0
			m.UpdateTables()
		case "ctrl+a":
			m.searchField.Blur()
			m.view = viewLibrary
			m.currentPage, m.selectedID = 0, 0
			m.UpdateTables()
		case "ctrl+s":
			m.view = viewSearch
			m.searchField.SetValue("")
			m.searchField.Focus()
		case "d":
			if m.view == viewTorrents {
				m.torrents[m.selectedID].DownloadStatus = "pending"
				m.Downloading = append(m.Downloading, m.torrents[m.selectedID])
				m.UpdateTables()
				return m, m.DownloadTorrents()
			}
		case "x":
			if m.view == viewDownloads {
				m.cancelDownload()
				return m, nil
			} else if m.view == viewLibrary {
				m.removeItem(m.Library[m.selectedID].Name, "L")
				return m, nil
			}
		case "enter":
			if m.view == viewSearch {
				m.search = m.searchField.Value()
				torrents, err := SearchTorrents(m.search)
				if err != nil {
					m.err = err
				} else {
					m.torrents = torrents
					m.UpdateTables()
					m.view = viewTorrents
					m.currentPage, m.selectedID = 0, 0
					m.searchField.Blur()
				}
			}
		case "right", "left", "down", "up":
			m.handleNavigation(msg.String())
		}
	case struct{}:
		return m, tick()
	}

	m.searchField, cmd = m.searchField.Update(msg)
	return m, cmd
}
func (m *model) handleNavigation(key string) {
	var length int

	switch m.view {
	case viewTorrents:
		length = len(m.torrents)
	case viewDownloads:
		length = len(m.Downloading)
	case viewLibrary:
		length = len(m.Library)
	default:
		return
	}

	switch key {
	case "right":
		if m.view == viewTorrents || m.view == viewDownloads || m.view == viewLibrary {
			if (m.currentPage+1)*m.rowsPerPage < length {
				m.currentPage++
				m.selectedID = m.currentPage * m.rowsPerPage
			}
		}
	case "left":
		if m.view == viewTorrents || m.view == viewDownloads || m.view == viewLibrary {
			if m.currentPage > 0 {
				m.currentPage--
				m.selectedID = m.currentPage * m.rowsPerPage
			}
		}
	case "down":
		if m.view == viewTorrents || m.view == viewDownloads || m.view == viewLibrary {
			if m.selectedID < length-1 {
				m.selectedID++
				if m.selectedID >= (m.currentPage+1)*m.rowsPerPage {
					m.currentPage++
				}
			}
		}
	case "up":
		if m.view == viewTorrents || m.view == viewDownloads || m.view == viewLibrary {
			if m.selectedID > 0 {
				m.selectedID--
				if m.selectedID < m.currentPage*m.rowsPerPage {
					m.currentPage--
				}
			}
		}
	}
}

func (m model) getRowStyle(index int) lipgloss.Style {
	if m.selectedID == index {
		return m.styles.SelectedRow
	}
	return lipgloss.NewStyle()
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("An error occurred: %v\n", m.err)
	}

	var titleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#5e81ac")).
		Bold(true)

	var header string
	switch m.view {
	case viewSearch:
		header = titleStyle.Render("Search Torrents")
	case viewTorrents:
		header = titleStyle.
			Align(lipgloss.Center).
			Width(m.width / 2).
			Render("Available Torrents")
	case viewDownloads:
		header = titleStyle.Render("Current Downloads")
	case viewLibrary:
		header = titleStyle.Render("Library")
	default:
		header = ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, m.renderContent())
}

func (m model) renderContent() string {
	switch m.view {
	case viewSearch:
		return m.renderSearchView()
	case viewTorrents:
		return m.renderTorrentsView()
	case viewDownloads:
		return m.renderDownloadsView()
	case viewLibrary:
		return m.renderLibrary()
	default:
		return ""
	}
}

func (m model) renderSearchView() string {
	searchField := m.styles.InputField.Render(m.searchField.View())
	return lipgloss.Place(
		m.width/2+25, m.height,
		lipgloss.Right, lipgloss.Center,
		searchField,
	)
}

func (m model) renderTorrentsView() string {
	start, end := m.currentPage*m.rowsPerPage, (m.currentPage+1)*m.rowsPerPage
	if end > len(m.torrents) {
		end = len(m.torrents)
	}

	visibleTorrents := m.torrents[start:end]
	rows := make([]table.Row, len(visibleTorrents))

	for i, torrent := range visibleTorrents {
		name := torrent.Name
		if len(name) > 50 {
			name = name[:47] + "..."
		}
		row := table.NewRow(table.RowData{
			"name":      name,
			"size":      torrent.Size,
			"seeders":   torrent.Seeders,
			"leechers":  torrent.Leechers,
			"num_files": torrent.NumFiles,
		})

		if i+start == m.selectedID {
			row = row.WithStyle(m.styles.SelectedRow)
		}

		rows[i] = row
	}

	paginationFooter := lipgloss.NewStyle().
		Background(lipgloss.Color("#4c566a")).
		Foreground(lipgloss.Color("#eceff4")).
		Padding(0, 1).
		Render(fmt.Sprintf("Page %d/%d (Use ←/→ to navigate, ↑/↓ to select)",
			m.currentPage+1, (len(m.torrents)+m.rowsPerPage-1)/m.rowsPerPage))

	tableView := m.torrentTable.WithRows(rows).View()

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#81a1c1")).
		Bold(true).
		Underline(true).
		Padding(0, 1)

	header := headerStyle.Render(fmt.Sprintf("Results for: %s", m.search))

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Left,
			header,
			tableView,
			paginationFooter,
		),
	)
}

func (m *model) renderLibrary() string {
	cardWidth := 40
	var cards []string

	for i, torrent := range m.Library {
		cardStyle := lipgloss.NewStyle().
			Width(cardWidth).
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			Margin(1)
		if i == m.selectedID {
			cardStyle = cardStyle.BorderForeground(lipgloss.Color("#88c0d0")).Bold(true)
		} else {
			cardStyle = cardStyle.BorderForeground(lipgloss.Color("#5e81ac"))
		}
		cardContent := fmt.Sprintf("%s\nSize: %s", torrent.Name, torrent.Size)
		cards = append(cards, cardStyle.Render(cardContent))
	}

	cols := m.width / (cardWidth + 4)
	if cols < 1 {
		cols = 1
	}

	var rows []string
	for i := 0; i < len(cards); i += cols {
		end := i + cols
		if end > len(cards) {
			end = len(cards)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, cards[i:end]...)
		rows = append(rows, row)
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m model) renderDownloadsView() string {
	cardWidth := 40
	var cards []string

	for i, torrent := range m.Downloading {
		cardStyle := lipgloss.NewStyle().
			Width(cardWidth).
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			Margin(1)
		if i == m.selectedID {
			cardStyle = cardStyle.BorderForeground(lipgloss.Color("#88c0d0")).Bold(true)
		} else {
			cardStyle = cardStyle.BorderForeground(lipgloss.Color("#5e81ac"))
		}
		// Note: The ETA is stored in torrent.Time, which is set in GetDownloadInfo.
		cardContent := fmt.Sprintf("%s\nSize: %s\nDownloaded: %s\nSpeed: %s\nETA: %s",
			torrent.Name, torrent.Size, torrent.CompletedSize, torrent.DownloadSpeed, torrent.Time)
		cards = append(cards, cardStyle.Render(cardContent))
	}

	cols := m.width / (cardWidth + 4)
	if cols < 1 {
		cols = 1
	}

	var rows []string
	for i := 0; i < len(cards); i += cols {
		end := i + cols
		if end > len(cards) {
			end = len(cards)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, cards[i:end]...)
		rows = append(rows, row)
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m *model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.rowsPerPage = height - 5
}

func main() {
	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	search := New()
	app := tea.NewProgram(search, tea.WithAltScreen())
	app.Run()
}
