package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var trackers = []string{
	"udp://tracker.openbittorrent.com:80",
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://9.rarbg.to:2920/announce",
	"udp://tracker.internetwarriors.net:1337/announce",
	"udp://tracker.leechers-paradise.org:6969",
	"udp://tracker.coppersurfer.tk:6969/announce",
	"udp://exodus.desync.com:6969",
	"udp://open.stealth.si:80/announce",
	"udp://tracker.tiny-vps.com:6969/announce",
	"udp://tracker.cyberia.is:6969/announce",
	"udp://tracker.moeking.me:6969/announce",
}

const (
	aria2URL         = "http://localhost:%d/jsonrpc"
	aria2SecretToken = "zivotjelijp12345"
)

type Request struct {
	JSONRPC string   `json:"jsonrpc"`
	ID      string   `json:"id"`
	Method  string   `json:"method"`
	Params  []string `json:"params"`
}

type Response struct {
	Result []Torrent `json:"result"`
}

type Torrent struct {
	GID            string `json:"gid,omitempty"`
	DownloadStatus string
	PGID           int    `json:"PGID"`
	Status         string `json:"status"`
	Size           string `json:"size"`
	CompletedSize  string `json:"completedLength"`
	DownloadSpeed  string `json:"downloadSpeed"`
	Time           string
	InfoHash       string `json:"info_hash"`
	Name           string `json:"name"`
	Leechers       int    `json:"leechers"`
	Seeders        int    `json:"seeders"`
	NumFiles       int    `json:"num_files"`
	Port           int
}

func (m *model) cancelDownload() {
	t := &m.Downloading[m.selectedID]
	pgid := strconv.Itoa(t.PGID)
	cmd := exec.Command("pkill", "-g", pgid)

	if err := cmd.Start(); err != nil {
		log.Printf("couldn't execute command: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		log.Printf("couldn't kill process: %v", err)
	}

	dir := filepath.Join(homeDir, "Downloads", "Sailor", sanitizeFileName(t.Name))

	cmd = exec.Command("rm", "-rf", dir)

	if err := cmd.Start(); err != nil {
		log.Printf("couldn't execute command: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		log.Printf("couldn't clean up torrent: %v", err)
	}

	m.removeItem(m.Downloading[m.selectedID].Name, "D")
}

func (m *model) removeItem(name string, source string) {
	if source == "D" {
		dir := filepath.Join(homeDir, "Downloads", "Sailor", sanitizeFileName(name))
		cmd := exec.Command("rm", "-rf", dir)

		if err := cmd.Start(); err != nil {
			log.Printf("couldn't execute command: %v", err)
		}

		if err := cmd.Wait(); err != nil {
			log.Printf("couldn't clean up torrent: %v", err)
		}

		var new []Torrent
		for _, t := range m.Downloading {
			if t.Name != name {
				new = append(new, t)
			}
		}
		m.Downloading = new
	} else if source == "L" {
		dir := filepath.Join(homeDir, "Downloads", "Sailor", sanitizeFileName(name))
		cmd := exec.Command("rm", "-rf", dir)

		if err := cmd.Start(); err != nil {
			log.Printf("couldn't execute command: %v", err)
		}

		if err := cmd.Wait(); err != nil {
			log.Printf("couldn't clean up torrent: %v", err)
		}
		var new []Torrent
		for _, t := range m.Library {
			if t.Name != name {
				new = append(new, t)
			}
		}
		m.Library = new
	}
}

func SearchTorrents(search string) ([]Torrent, error) {
	apiURL := fmt.Sprintf("https://apibay.org/q.php?q=%s", search)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Failed to fetch data %s: ", resp.Status)
	}

	var tempTorrents []struct {
		InfoHash string `json:"info_hash"`
		Name     string `json:"name"`
		Size     string `json:"size"`
		Leechers string `json:"leechers"`
		Seeders  string `json:"seeders"`
		NumFiles string `json:"num_files"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tempTorrents); err != nil {
		return nil, err
	}

	var torrents []Torrent
	for _, t := range tempTorrents {
		leechers, _ := strconv.Atoi(t.Leechers)
		seeders, _ := strconv.Atoi(t.Seeders)
		numFiles, _ := strconv.Atoi(t.NumFiles)
		size := formatSize(t.Size)

		torrents = append(torrents, Torrent{
			InfoHash: t.InfoHash,
			Name:     t.Name,
			Size:     size,
			Leechers: leechers,
			Seeders:  seeders,
			NumFiles: numFiles,
		})
	}

	return torrents, nil
}

func getFreePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

func (m *model) DownloadTorrents() tea.Cmd {
	return func() tea.Msg {
		for i := range m.Downloading {
			t := &m.Downloading[i]
			if t.DownloadStatus == "pending" {
				safeName := sanitizeFileName(t.Name)
				downloadDir := filepath.Join(homeDir, "Downloads", "Sailor", safeName)

				if err := os.MkdirAll(downloadDir, 0755); err != nil {
					log.Printf("Error creating download directory: %v", err)
					continue
				}

				var err error
				t.Port, err = getFreePort()
				if err != nil {
					log.Printf("Couldn't find free port: %v", err)
					continue
				}

				magnetLink := CreateMagnetLink(t.InfoHash, t.Name)

				cmd := exec.Command("aria2c", "--enable-rpc=true",
					fmt.Sprintf("--rpc-listen-port=%d", t.Port),
					"--dir", downloadDir,
					magnetLink)

				cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

				log.Printf("Running command: %s", strings.Join(cmd.Args, " "))
				if err := cmd.Start(); err != nil {
					log.Printf("Failed to start aria2c: %v", err)
					continue
				}

				t.PGID, err = syscall.Getpgid(cmd.Process.Pid)
				t.DownloadStatus = "Downloading"
				if err != nil {
					log.Printf("Cannot get process GID: %v", err)
					continue
				}

				go func(t *Torrent) {
					if err := cmd.Wait(); err != nil {
						t.DownloadStatus = "Failed"
						log.Printf("aria2c finished with error: %v", err)
					} else {
						t.DownloadStatus = "Complete"
					}
				}(t)
			}
		}
		return downloadCreateMsg{}
	}
}

func sanitizeFileName(name string) string {
	return strings.ReplaceAll(name, " ", "_")
}

func CreateMagnetLink(infoHash string, name string) string {
	magnet := fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s&tr=", infoHash, name)
	for _, tracker := range trackers {
		magnet += "&tr=" + url.QueryEscape(tracker)
	}
	return magnet
}

func (m *model) GetDownloadInfo() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for i := range m.Downloading {
			t := &m.Downloading[i]
			if t.DownloadStatus == "Downloading" {
				go func(t *Torrent) {
					downloadInfo, err := FetchDownloadInfo(t.Port)
					if err != nil {
						log.Printf("Error fetching download info for %s (Port: %d): %v", t.Name, t.Port, err)
						return
					}

					if len(downloadInfo) > 0 {
						download := downloadInfo[0]
						t.Status = download.Status
						t.CompletedSize = formatSize(download.CompletedSize)
						t.DownloadSpeed = formatSpeed(download.DownloadSpeed)
						log.Printf("• %s\nSize: %s\nDownloaded: %s\nSpeed: %s\nStatus: %s\nTime: %s\n",
							t.Name, t.Size, t.CompletedSize, t.DownloadSpeed, t.Status, t.Time)

						if t.Size == t.CompletedSize {
							t.DownloadStatus = "Complete"
						}
					} else {
						log.Printf("No download info received for %s (Port: %d)", t.Name, t.Port)
					}
				}(t)
			} else if t.DownloadStatus == "Complete" {
				t.DownloadStatus = "Stored"
				//m.removeItem(t.Name, "D") // #FIX I changed this function and now it deletes the files instead of just the Torrent from the downloads
				// While you're at it restructure fetched info so you can calculate ETA like a normal huma being
				m.Library = append(m.Library, *t)
			}
		}
	}
}

func FetchDownloadInfo(port int) ([]Torrent, error) {
	log.Printf("Fetching download info on port: %d", port)

	req := Request{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "aria2.tellActive",
		Params:  []string{"token:" + aria2SecretToken},
	}

	resp, err := sendRequest(req, port)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return nil, err
	}

	return resp.Result, nil
}

func sendRequest(req Request, port int) (*Response, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(fmt.Sprintf(aria2URL, port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to send request: %s", resp.Status)
	}

	var response Response
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return &response, nil
}

func formatSize(bytes string) string {
	bytesInt, err := strconv.ParseInt(bytes, 10, 64)
	if err != nil {
		return "N/A"
	}

	if bytesInt < 1024*1024 {
		return fmt.Sprintf("%.2f KB", float64(bytesInt)/1024)
	} else if bytesInt < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(bytesInt)/(1024*1024))
	} else {
		return fmt.Sprintf("%.2f GB", float64(bytesInt)/(1024*1024*1024))
	}
}

func formatSpeed(bytesPerSec string) string {
	bytesInt, err := strconv.ParseInt(bytesPerSec, 10, 64)
	if err != nil {
		return "N/A"
	}

	if bytesInt < 1024 {
		return fmt.Sprintf("%d B/s", bytesInt)
	} else if bytesInt < 1024*1024 {
		return fmt.Sprintf("%.2f KB/s", float64(bytesInt)/1024)
	} else {
		return fmt.Sprintf("%.2f MB/s", float64(bytesInt)/(1024*1024))
	}
}
