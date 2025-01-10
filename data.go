package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var (
	homeDir, _ = os.UserHomeDir()
	savePath   = filepath.Join(homeDir, "Downloads", "Sailor", ".downloading.json")
	saveMutex  sync.Mutex
)

func (m *model) saveDownloadState() error {
	saveMutex.Lock()
	defer saveMutex.Unlock()

	err := os.MkdirAll(filepath.Dir(savePath), 0755)
	if err != nil {
		return err
	}

	file, err := os.Create(savePath)
	if err != nil {
		return err
	}
	defer file.Close()

	allTorrents := append(m.Downloading, m.Library...)

	encoder := json.NewEncoder(file)
	err = encoder.Encode(allTorrents)
	if err != nil {
		return err
	}

	log.Println("Download state saved successfully.")
	return nil
}

func (m *model) loadDownloadState() error {
	saveMutex.Lock()
	defer saveMutex.Unlock()

	file, err := os.Open(savePath)
	if os.IsNotExist(err) {
		log.Println("Download state file does not exist. Starting fresh.")
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	var allTorrents []Torrent
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&allTorrents)
	if err != nil {
		return err
	}

	allTorrents = removeDuplicateTorrents(allTorrents)

	m.Downloading = nil
	m.Library = nil

	for _, t := range allTorrents {
		if t.DownloadStatus == "Downloading" {
			m.Downloading = append(m.Downloading, t)
		} else {
			m.Library = append(m.Library, t)
		}
	}

	log.Println("Download state loaded successfully.")

	return nil
}

func removeDuplicateTorrents(torrents []Torrent) []Torrent {
	seen := make(map[string]bool)
	var uniqueTorrents []Torrent
	for _, t := range torrents {
		if !seen[t.InfoHash] {
			seen[t.InfoHash] = true
			uniqueTorrents = append(uniqueTorrents, t)
		}
	}
	return uniqueTorrents
}

func checkAria2cRPC(port int) error {
	url := fmt.Sprintf("http://localhost:%d/jsonrpc", port)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
