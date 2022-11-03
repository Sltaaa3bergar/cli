package browse

import (
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/repo/view"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
)

// TODO factor out install/remove for testing
// TODO see if somehow loadSelectedReadme can be refactored to be testable (problem is the QueueUpdateDraw)

func Test_readmeGetter(t *testing.T) {
	reg := httpmock.Registry{}
	defer reg.Verify(t)

	content := base64.StdEncoding.EncodeToString([]byte("lol"))

	reg.Register(
		httpmock.REST("GET", "repos/vilmibm/gh-screensaver/readme"),
		httpmock.JSONResponse(view.RepoReadme{Content: content}))

	client := &http.Client{Transport: &reg}

	rg := newReadmeGetter(client, time.Second)

	readme, err := rg.Get("vilmibm/gh-screensaver")
	assert.NoError(t, err)

	assert.Equal(t, "lol", readme)
}

func Test_getExtensionRepos(t *testing.T) {
	reg := httpmock.Registry{}
	defer reg.Verify(t)

	client := &http.Client{Transport: &reg}

	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"100"},
		"q":        []string{"topic:gh-extension"},
	}
	cfg := config.NewBlankConfig()

	cfg.DefaultHostFunc = func() (string, string) { return "github.com", "" }

	reg.Register(
		httpmock.QueryMatcher("GET", "search/repositories", values),
		httpmock.JSONResponse(search.RepositoriesResult{
			IncompleteResults: false,
			Items: []search.Repository{
				{
					FullName:    "vilmibm/gh-screensaver",
					Name:        "gh-screensaver",
					Description: "terminal animations",
					Owner: search.User{
						Login: "vilmibm",
					},
				},
				{
					FullName:    "cli/gh-cool",
					Name:        "gh-cool",
					Description: "it's just cool ok",
					Owner: search.User{
						Login: "cli",
					},
				},
				{
					FullName:    "samcoe/gh-triage",
					Name:        "gh-triage",
					Description: "helps with triage",
					Owner: search.User{
						Login: "samcoe",
					},
				},
				{
					FullName:    "github/gh-gei",
					Name:        "gh-gei",
					Description: "something something enterprise",
					Owner: search.User{
						Login: "github",
					},
				},
			},
			Total: 4,
		}),
	)

	searcher := search.NewSearcher(client, "github.com")
	emMock := &extensions.ExtensionManagerMock{}
	emMock.ListFunc = func() []extensions.Extension {
		return []extensions.Extension{
			&extensions.ExtensionMock{
				URLFunc: func() string {
					return "https://github.com/vilmibm/gh-screensaver"
				},
			},
			&extensions.ExtensionMock{
				URLFunc: func() string {
					return "https://github.com/github/gh-gei"
				},
			},
		}
	}

	opts := ExtBrowseOpts{
		Searcher: searcher,
		Em:       emMock,
		Cfg:      cfg,
	}

	extEntries, err := getExtensions(opts)
	assert.NoError(t, err)

	expectedEntries := []extEntry{
		{
			URL:         "https://github.com/vilmibm/gh-screensaver",
			Name:        "gh-screensaver",
			FullName:    "vilmibm/gh-screensaver",
			Installed:   true,
			Official:    false,
			description: "terminal animations",
		},
		{
			URL:         "https://github.com/cli/gh-cool",
			Name:        "gh-cool",
			FullName:    "cli/gh-cool",
			Installed:   false,
			Official:    true,
			description: "it's just cool ok",
		},
		{
			URL:         "https://github.com/samcoe/gh-triage",
			Name:        "gh-triage",
			FullName:    "samcoe/gh-triage",
			Installed:   false,
			Official:    false,
			description: "helps with triage",
		},
		{
			URL:         "https://github.com/github/gh-gei",
			Name:        "gh-gei",
			FullName:    "github/gh-gei",
			Installed:   true,
			Official:    true,
			description: "something something enterprise",
		},
	}

	assert.Equal(t, expectedEntries, extEntries)
}

func Test_extEntry(t *testing.T) {
	cases := []struct {
		name          string
		ee            extEntry
		expectedTitle string
		expectedDesc  string
	}{
		{
			name: "official",
			ee: extEntry{
				Name:        "gh-cool",
				FullName:    "cli/gh-cool",
				Installed:   false,
				Official:    true,
				description: "it's just cool ok",
			},
			expectedTitle: "cli/gh-cool [yellow](official)",
			expectedDesc:  "it's just cool ok",
		},
		{
			name: "installed",
			ee: extEntry{
				Name:        "gh-screensaver",
				FullName:    "vilmibm/gh-screensaver",
				Installed:   true,
				Official:    false,
				description: "animations in your terminal",
			},
			expectedTitle: "vilmibm/gh-screensaver [green](installed)",
			expectedDesc:  "animations in your terminal",
		},
		{
			name: "neither",
			ee: extEntry{
				Name:        "gh-triage",
				FullName:    "samcoe/gh-triage",
				Installed:   false,
				Official:    false,
				description: "help with triage",
			},
			expectedTitle: "samcoe/gh-triage",
			expectedDesc:  "help with triage",
		},
		{
			name: "both",
			ee: extEntry{
				Name:        "gh-gei",
				FullName:    "github/gh-gei",
				Installed:   true,
				Official:    true,
				description: "something something enterprise",
			},
			expectedTitle: "github/gh-gei [yellow](official) [green](installed)",
			expectedDesc:  "something something enterprise",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedTitle, tt.ee.Title())
			assert.Equal(t, tt.expectedDesc, tt.ee.Description())
		})
	}
}

func Test_extList(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	list := tview.NewList()
	extEntries := []extEntry{
		{
			Name:        "gh-cool",
			FullName:    "cli/gh-cool",
			Installed:   false,
			Official:    true,
			description: "it's just cool ok",
		},
		{
			Name:        "gh-screensaver",
			FullName:    "vilmibm/gh-screensaver",
			Installed:   true,
			Official:    false,
			description: "animations in your terminal",
		},
		{
			Name:        "gh-triage",
			FullName:    "samcoe/gh-triage",
			Installed:   false,
			Official:    false,
			description: "help with triage",
		},
		{
			Name:        "gh-gei",
			FullName:    "github/gh-gei",
			Installed:   true,
			Official:    true,
			description: "something something enterprise",
		},
	}
	app := tview.NewApplication()

	extList := newExtList(app, list, extEntries, logger)

	extList.Filter("cool")
	assert.Equal(t, 1, extList.list.GetItemCount())

	title, _ := extList.list.GetItemText(0)
	assert.Equal(t, "cli/gh-cool [yellow](official)", title)

	extList.ToggleInstalled(0)
	assert.True(t, extList.extEntries[0].Installed)

	extList.Refresh()
	assert.Equal(t, 1, extList.list.GetItemCount())

	title, _ = extList.list.GetItemText(0)
	assert.Equal(t, "cli/gh-cool [yellow](official) [green](installed)", title)

	extList.Reset()
	assert.Equal(t, 4, extList.list.GetItemCount())

	ee, ix := extList.FindSelected()
	assert.Equal(t, 0, ix)
	assert.Equal(t, "cli/gh-cool [yellow](official) [green](installed)", ee.Title())

	extList.ScrollDown()
	ee, ix = extList.FindSelected()
	assert.Equal(t, 1, ix)
	assert.Equal(t, "vilmibm/gh-screensaver [green](installed)", ee.Title())

	extList.ScrollUp()
	ee, ix = extList.FindSelected()
	assert.Equal(t, 0, ix)
	assert.Equal(t, "cli/gh-cool [yellow](official) [green](installed)", ee.Title())

	extList.PageDown()
	ee, ix = extList.FindSelected()
	assert.Equal(t, 3, ix)
	assert.Equal(t, "github/gh-gei [yellow](official) [green](installed)", ee.Title())

	extList.PageUp()
	ee, ix = extList.FindSelected()
	assert.Equal(t, 0, ix)
	assert.Equal(t, "cli/gh-cool [yellow](official) [green](installed)", ee.Title())
}