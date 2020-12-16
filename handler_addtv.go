package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/drakenot/gramarr/sonarr"

	"path/filepath"

	tb "gopkg.in/tucnak/telebot.v2"
)

func (e *Env) HandleAddTVShow(m *tb.Message) {
	e.CM.StartConversation(NewAddTVShowConversation(e), m)
}

func NewAddTVShowConversation(e *Env) *AddTVShowConversation {
	return &AddTVShowConversation{env: e}
}

type AddTVShowConversation struct {
	currentStep             Handler
	TVQuery                 string
	TVShowResults           []sonarr.TVShow
	folderResults           []sonarr.Folder
	selectedTVShow          *sonarr.TVShow
	selectedTVShowSeasons   []*sonarr.TVShowSeason
	selectedLanguageProfile *sonarr.Profile
	selectedFolder          *sonarr.Folder
	env                     *Env
}

func (c *AddTVShowConversation) Run(m *tb.Message) {
	c.currentStep = c.AskTVShow(m)
}

func (c *AddTVShowConversation) Name() string {
	return "addtv"
}

func (c *AddTVShowConversation) CurrentStep() Handler {
	return c.currentStep
}

func (c *AddTVShowConversation) AskTVShow(m *tb.Message) Handler {
	Send(c.env.Bot, m.Sender, "What TV Show do you want to search for?")

	return func(m *tb.Message) {
		c.TVQuery = m.Text

		TVShows, err := c.env.Sonarr.SearchTVShows(c.TVQuery)
		c.TVShowResults = TVShows

		// Search Service Failed
		if err != nil {
			fmt.Println(err)
			SendError(c.env.Bot, m.Sender, "Failed to search TV Show.")
			c.env.CM.StopConversation(c)
			return
		}

		// No Results
		if len(TVShows) == 0 {
			msg := fmt.Sprintf("No TV Show found with the title '%s'", EscapeMarkdown(c.TVQuery))
			Send(c.env.Bot, m.Sender, msg)
			c.env.CM.StopConversation(c)
			return
		}

		// Found some TVShows! Yay!
		var msg []string
		msg = append(msg, fmt.Sprintf("*Found %d TV Shows:*", len(TVShows)))
		for i, TV := range TVShows {
			msg = append(msg, fmt.Sprintf("%d) %s", i+1, EscapeMarkdown(TV.String())))
		}
		Send(c.env.Bot, m.Sender, strings.Join(msg, "\n"))
		c.currentStep = c.AskPickTVShow(m)
	}
}

func (c *AddTVShowConversation) AskPickTVShow(m *tb.Message) Handler {

	// Send custom reply keyboard
	var options []string
	for _, TVShow := range c.TVShowResults {
		options = append(options, fmt.Sprintf("%s", TVShow))
	}
	options = append(options, "/cancel")
	SendKeyboardList(c.env.Bot, m.Sender, "Which one would you like to download?", options)

	return func(m *tb.Message) {

		// Set the selected TVShow
		for i := range options {
			if m.Text == options[i] {
				c.selectedTVShow = &c.TVShowResults[i]
				break
			}
		}

		// Not a valid TV selection
		if c.selectedTVShow == nil {
			SendError(c.env.Bot, m.Sender, "Invalid selection.")
			c.currentStep = c.AskPickTVShow(m)
			return
		}

		// Mark all seasons unmonitored from lookup
		for i := 0; i < len(c.selectedTVShow.Seasons); i++ {
			c.selectedTVShow.Seasons[i].Monitored = false
		}

		// merge seasons from the show if it already exists in Sonarr
		var currentShows, err = c.env.Sonarr.GetTVShows()

		if currentShows != nil && err == nil {
			for _, show := range currentShows {
				if show.TVDBID == c.selectedTVShow.TVDBID {
					c.selectedTVShow.Seasons = show.Seasons
					break
				}
			}
		}

		c.currentStep = c.AskPickTVShowSeason(m)
	}
}

func (c *AddTVShowConversation) isSelectedSeason(s *sonarr.TVShowSeason) bool {

	for _, season := range c.selectedTVShowSeasons {
		if s.SeasonNumber == season.SeasonNumber {
			return true
		}
	}

	return false
}

func (c *AddTVShowConversation) AskPickTVShowSeason(m *tb.Message) Handler {

	if c.selectedTVShowSeasons == nil {
		c.selectedTVShowSeasons = []*sonarr.TVShowSeason{}
	}

	// Send custom reply keyboard
	var options []string
	if len(c.selectedTVShowSeasons) > 0 {
		options = append(options, "Nope. I'm done!")
	}

	var seasonCount int = 0
	for _, season := range c.selectedTVShow.Seasons {
		if !c.isSelectedSeason(season) && season.SeasonNumber > 0 && !season.Monitored {
			options = append(options, fmt.Sprintf("%v", season.SeasonNumber))
			seasonCount++
		}
	}

	if seasonCount == 0 && len(c.selectedTVShowSeasons) == 0 {
		SendError(c.env.Bot, m.Sender, "No seasons left in this show to add.")
		c.env.CM.StopConversation(c)
		return nil
	}

	options = append(options, "/cancel")

	if len(c.selectedTVShowSeasons) > 0 {
		SendKeyboardList(c.env.Bot, m.Sender, "Any other season?", options)
	} else {
		SendKeyboardList(c.env.Bot, m.Sender, "Which season would you like to download?", options)
	}

	return func(m *tb.Message) {

		if m.Text == "Nope. I'm done!" {

			// merge selected seasons
			// for i := 0; i < len(c.selectedTVShow.Seasons); i++ {
			// 	for j := 0; j < len(c.selectedTVShowSeasons); j++ {
			// 		if c.selectedTVShowSeasons[j].SeasonNumber != c.selectedTVShow.Seasons[i].SeasonNumber {
			// 			continue
			// 		}

			// 		if c.selectedTVShowSeasons[j].Monitored {
			// 			c.selectedTVShow.Seasons[i].Monitored = true
			// 		}
			// 	}
			// }

			// c.selectedTVShow.Seasons = c.selectedTVShowSeasons

			c.currentStep = c.AskFolder(m)

			return
		} else {
			var selectedSeason *sonarr.TVShowSeason

			// Set the selected TV
			i, err := strconv.Atoi(m.Text)
			if err == nil && i <= len(c.selectedTVShow.Seasons) && i > 0 {
				for _, season := range c.selectedTVShow.Seasons {
					if i == season.SeasonNumber {
						selectedSeason = season
						break
					}
				}
			}

			// Not a valid TV selection
			if selectedSeason == nil {
				SendError(c.env.Bot, m.Sender, "Invalid selection.")
				c.currentStep = c.AskPickTVShowSeason(m)
				return
			}

			c.selectedTVShowSeasons = append(c.selectedTVShowSeasons, selectedSeason)
			c.currentStep = c.AskPickTVShowSeason(m)
		}
	}
}

func (c *AddTVShowConversation) AskFolder(m *tb.Message) Handler {

	folders, err := c.env.Sonarr.GetFolders()
	c.folderResults = folders

	// GetFolders Service Failed
	if err != nil {
		SendError(c.env.Bot, m.Sender, "Failed to get folders.")
		c.env.CM.StopConversation(c)
		return nil
	}

	// No Results
	if len(folders) == 0 {
		SendError(c.env.Bot, m.Sender, "No destination folders found.")
		c.env.CM.StopConversation(c)
		return nil
	}

	// Found folders!

	// Send the results
	var msg []string
	msg = append(msg, fmt.Sprintf("*Found %d folders:*", len(folders)))
	for i, folder := range folders {
		msg = append(msg, fmt.Sprintf("%d) %s", i+1, EscapeMarkdown(filepath.Base(folder.Path))))
	}
	Send(c.env.Bot, m.Sender, strings.Join(msg, "\n"))

	// Send the custom reply keyboard
	var options []string
	for _, folder := range folders {
		options = append(options, fmt.Sprintf("%s", filepath.Base(folder.Path)))
	}
	options = append(options, "/cancel")
	SendKeyboardList(c.env.Bot, m.Sender, "Which folder should it download to?", options)

	return func(m *tb.Message) {
		// Set the selected folder
		for i, opt := range options {
			if m.Text == opt {
				c.selectedFolder = &c.folderResults[i]
				break
			}
		}

		// Not a valid folder selection
		if c.selectedTVShow == nil {
			SendError(c.env.Bot, m.Sender, "Invalid selection.")
			c.currentStep = c.AskFolder(m)
			return
		}

		c.AddTVShow(m)
	}
}

func (c *AddTVShowConversation) AddTVShow(m *tb.Message) {
	_, err := c.env.Sonarr.AddTVShow(*c.selectedTVShow, c.selectedTVShowSeasons, c.env.Config.Sonarr.QualityID, c.selectedFolder.Path)

	// Failed to add TV
	if err != nil {
		SendError(c.env.Bot, m.Sender, "Failed to add TV.")
		c.env.CM.StopConversation(c)
		return
	}

	if c.selectedTVShow.PosterURL != "" {
		photo := &tb.Photo{File: tb.FromURL(c.selectedTVShow.PosterURL)}
		c.env.Bot.Send(m.Sender, photo)
	}

	// Notify User
	Send(c.env.Bot, m.Sender, "TV Show has been added!")

	// Notify Admin
	adminMsg := fmt.Sprintf("%s added TV Show '%s'", DisplayName(m.Sender), EscapeMarkdown(c.selectedTVShow.String()))
	SendAdmin(c.env.Bot, c.env.Users.Admins(), adminMsg)

	c.env.CM.StopConversation(c)
}
