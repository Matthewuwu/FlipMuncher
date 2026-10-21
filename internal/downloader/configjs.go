// Derived from anyflip-downloader (github.com/Lofter1/anyflip-downloader),
// Copyright (C) Lofter1 and contributors, licensed under the GNU General
// Public License v3.0. Modified 2026: uses a context-aware request
// instead of http.Get. See LICENSE-anyflip-downloader for the original
// license text.

package downloader

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

func getPageFileNames(configjs string) []string {
	r := regexp.MustCompile(`"n":\[".*?"\]`)
	matches := r.FindAllString(configjs, -1)

	for i, match := range matches {
		replacer := strings.NewReplacer("[", "", "\"", "", "]", "")
		match = strings.Split(match, ":")[1]
		matches[i] = replacer.Replace(match)
	}

	return matches
}

func getBookTitle(configjs string) (string, error) {
	r := regexp.MustCompile(`("?(bookConfig.)?bookTitle"?[=]"(.*?)")|"title":"(.*?)"`)
	m := r.FindStringSubmatch(configjs)
	if m == nil {
		return "", errors.New("could not find book title")
	}
	if m[3] != "" {
		return m[3], nil
	}
	return m[4], nil
}

func getPageCount(configjs string) (int, error) {
	r := regexp.MustCompile(`"?(bookConfig\.)?(total)?[Pp]ageCount"?[=:]"?\d+"?`)
	match := r.FindString(configjs)

	if strings.Contains(match, "=") {
		match = strings.Split(match, "=")[1]
	} else if strings.Contains(match, ":") {
		match = strings.Split(match, ":")[1]
	} else {
		return 0, errors.New("could not find page count")
	}

	match = strings.ReplaceAll(match, "\"", "")
	return strconv.Atoi(match)
}

func downloadConfigJS(ctx context.Context, client *http.Client, bookURL *url.URL) (string, error) {
	configjsURL, err := url.Parse("https://online.anyflip.com")
	if err != nil {
		return "", err
	}
	configjsURL.Path = path.Join(bookURL.Path, "mobile", "javascript", "config.js")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, configjsURL.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", errors.New("book not found (is the link correct and public?)")
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("received non-200 response: " + resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
