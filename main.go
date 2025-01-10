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
	downloadTable  table.Model
	libraryTable   table.Model
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
		rowsPerPage:    30,
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

func (m *model) CreateDownloadRows() []table.Row {
	rows := make([]table.Row, len(m.Downloading))
	for i, torrent := range m.Downloading {
		rows[i] = table.NewRow(table.RowData{
			"name":       torrent.Name,
			"size":       torrent.Size,
			"downloaded": torrent.CompletedSize,
			"speed":      torrent.DownloadSpeed,
			"time":       torrent.Time,
		})
	}
	return rows
}

func (m *model) CreateLibraryRows() []table.Row {
	rows := make([]table.Row, len(m.Library))
	for i, torrent := range m.Library {
		rows[i] = table.NewRow(table.RowData{
			"name": torrent.Name,
			"size": torrent.Size,
		})
	}
	return rows
}

func InitTable(columns []table.Column, rows []table.Row) table.Model {
	return table.New(columns).WithRows(rows).SortByDesc("seeders")
}

func CreateTorrentColumns() []table.Column {
	return []table.Column{
		table.NewColumn("name", "Name", 62),
		table.NewColumn("size", "Size", 9),
		table.NewColumn("seeders", "Seeders", 8),
		table.NewColumn("leechers", "Leechers", 8),
		table.NewColumn("num_files", "Files", 6),
	}
}

func CreateDownloadColumns() []table.Column {
	return []table.Column{
		table.NewColumn("name", "Name", 62),
		table.NewColumn("size", "Size", 9),
		table.NewColumn("downloaded", "Downloaded", 9),
		table.NewColumn("speed", "Speed", 9),
		table.NewColumn("time", "Time", 6),
	}
}

func CreateLibraryColumns() []table.Column {
	return []table.Column{
		table.NewColumn("name", "Name", 50),
		table.NewColumn("size", "Size", 10),
	}
}

func (m *model) UpdateTables() {
	m.torrentTable = InitTable(CreateTorrentColumns(), m.CreateTorrentRows(m.torrents))
	m.downloadTable = InitTable(CreateDownloadColumns(), m.CreateDownloadRows())
	m.libraryTable = InitTable(CreateLibraryColumns(), m.CreateLibraryRows())
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
		if (m.currentPage+1)*m.rowsPerPage < length {
			m.currentPage++
			m.selectedID = m.currentPage * m.rowsPerPage
		}
	case "left":
		if m.currentPage > 0 {
			m.currentPage--
			m.selectedID = m.currentPage * m.rowsPerPage
		}
	case "down":
		if m.selectedID < length-1 {
			m.selectedID++
			if m.selectedID >= (m.currentPage+1)*m.rowsPerPage {
				m.currentPage++
			}
		}
	case "up":
		if m.selectedID > 0 {
			m.selectedID--
			if m.selectedID < m.currentPage*m.rowsPerPage {
				m.currentPage--
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
		header = titleStyle.Render("Available Torrents")
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

func (m *model) renderLibrary() string {
	start, end := 0, len(m.Library)
	if end > len(m.Library) {
		end = len(m.Library)
	}

	visibleDownloads := m.Library[start:end]
	rows := make([]table.Row, len(visibleDownloads))

	for i, torrent := range visibleDownloads {
		row := table.NewRow(table.RowData{
			"name": torrent.Name,
			"size": torrent.Size,
		})

		if i+start == m.selectedID {
			row = row.WithStyle(m.styles.SelectedRow)
		}

		rows[i] = row
	}

	tableView := m.libraryTable.WithRows(rows).View()

	paginationFooter := lipgloss.NewStyle().
		Background(lipgloss.Color("#4c566a")).
		Foreground(lipgloss.Color("#eceff4")).
		Padding(0, 1).
		Render(fmt.Sprintf("Page %d/%d (Use ←/→ to navigate, ↑/↓ to select)",
			m.currentPage+1, (len(m.torrents)+m.rowsPerPage-1)/m.rowsPerPage))

	return lipgloss.JoinVertical(lipgloss.Left,
		tableView,
		paginationFooter,
	)
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
		row := table.NewRow(table.RowData{
			"name":      torrent.Name,
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

	return lipgloss.Place(
		m.width/2+47, m.height,
		lipgloss.Right, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Left,
			"Results for: "+m.search,
			tableView,
			paginationFooter,
		),
	)

}

func (m model) renderDownloadsView() string {
	start, end := 0, len(m.Downloading)
	if end > len(m.Downloading) {
		end = len(m.Downloading)
	}

	visibleDownloads := m.Downloading[start:end]
	rows := make([]table.Row, len(visibleDownloads))

	for i, torrent := range visibleDownloads {
		row := table.NewRow(table.RowData{
			"name":       torrent.Name,
			"size":       torrent.Size,
			"downloaded": torrent.CompletedSize,
			"speed":      torrent.DownloadSpeed,
			"time":       torrent.Time,
		})

		if i+start == m.selectedID {
			row = row.WithStyle(m.styles.SelectedRow)
		}

		rows[i] = row
	}

	tableView := m.downloadTable.WithRows(rows).View()

	paginationFooter := lipgloss.NewStyle().
		Background(lipgloss.Color("#4c566a")).
		Foreground(lipgloss.Color("#eceff4")).
		Padding(0, 1).
		Render(fmt.Sprintf("Page %d/%d (Use ←/→ to navigate, ↑/↓ to select)",
			m.currentPage+1, (len(m.torrents)+m.rowsPerPage-1)/m.rowsPerPage))

	return lipgloss.JoinVertical(lipgloss.Left,
		tableView,
		paginationFooter,
	)
}

func (m *model) SetSize(width, height int) {
	m.width = width
	m.height = height
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
