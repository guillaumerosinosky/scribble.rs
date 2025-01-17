package frontend

import (
	"log"
	"net/http"
	"strings"

	"github.com/guillaumerosinosky/scribble.rs/api"
	"github.com/guillaumerosinosky/scribble.rs/state"
	"github.com/guillaumerosinosky/scribble.rs/translations"
	"golang.org/x/text/language"
)

type lobbyPageData struct {
	*BasePageConfig
	*api.LobbyData

	Translation translations.Translation
	Locale      string
}

type robotPageData struct {
	*BasePageConfig
	*api.LobbyData
}

// ssrEnterLobby opens a lobby, either opening it directly or asking for a lobby.
func ssrEnterLobby(w http.ResponseWriter, r *http.Request) {
	state.LoadLobbies()
	lobby, err := api.GetLobby(r)
	if err != nil {
		userFacingError(w, err.Error())
		return
	}

	userAgent := strings.ToLower(r.UserAgent())
	if !(strings.Contains(userAgent, "gecko") || strings.Contains(userAgent, "chrome") || strings.Contains(userAgent, "opera") || strings.Contains(userAgent, "safari") || strings.Contains(userAgent, "go-http")) {
		templatingError := pageTemplates.ExecuteTemplate(w, "robot-page", &robotPageData{
			BasePageConfig: currentBasePageConfig,
			LobbyData:      api.CreateLobbyData(lobby),
		})
		if templatingError != nil {
			http.Error(w, "error templating robot page", http.StatusInternalServerError)
		}
		return
	}

	translation, locale := determineTranslation(r)
	requestAddress := api.GetIPAddressFromRequest(r)

	var pageData *lobbyPageData
	lobby.Synchronized(func() {
		lobby.WriteJSON = api.WriteJSON
		player := api.GetPlayer(lobby, r)

		if player == nil {
			if !lobby.HasFreePlayerSlot() {
				userFacingError(w, "Sorry, but the lobby is full.")
				return
			}

			var clientsWithSameIP int
			for _, otherPlayer := range lobby.GetPlayers() {
				if otherPlayer.GetLastKnownAddress() == requestAddress {
					clientsWithSameIP++
					if clientsWithSameIP >= lobby.ClientsPerIPLimit {
						userFacingError(w, "Sorry, but you have exceeded the maximum number of clients per IP.")
						return
					}
				}
			}

			newPlayer := lobby.JoinPlayer(api.GetPlayername(r))

			// Use the players generated usersession and pass it as a cookie.
			http.SetCookie(w, &http.Cookie{
				Name:     "usersession",
				Value:    newPlayer.GetUserSession(),
				Path:     "/",
				SameSite: http.SameSiteStrictMode,
			})
		} else {
			if player.Connected && player.GetWebsocket() != nil {
				userFacingError(w, "It appears you already have an open tab for this lobby.")
				return
			}
			player.SetLastKnownAddress(requestAddress)
		}

		pageData = &lobbyPageData{
			BasePageConfig: currentBasePageConfig,
			LobbyData:      api.CreateLobbyData(lobby),
			Translation:    translation,
			Locale:         locale,
		}
	})

	//If the pagedata isn't initialized, it means the synchronized block has exited.
	//In this case we don't want to tempalte the lobby, since an error has occurred
	//and probably already has been handled.
	if pageData != nil {
		templateError := pageTemplates.ExecuteTemplate(w, "lobby-page", pageData)
		if templateError != nil {
			log.Printf("Error templating lobby: %s\n", templateError)
			http.Error(w, "error templating lobby", http.StatusInternalServerError)
		}
	}
}

func determineTranslation(r *http.Request) (translations.Translation, string) {
	var translation translations.Translation

	languageTags, _, languageParseError := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if languageParseError == nil {
		for _, languageTag := range languageTags {
			fullLanguageIdentifier := languageTag.String()
			fullLanguageIdentifierLowercased := strings.ToLower(fullLanguageIdentifier)
			translation = translations.GetLanguage(fullLanguageIdentifierLowercased)
			if translation != nil {
				return translation, fullLanguageIdentifierLowercased
			}

			baseLanguageIdentifier, _ := languageTag.Base()
			baseLanguageIdentifierLowercased := strings.ToLower(baseLanguageIdentifier.String())
			translation = translations.GetLanguage(baseLanguageIdentifierLowercased)
			if translation != nil {
				return translation, baseLanguageIdentifierLowercased
			}
		}
	}

	return translations.DefaultTranslation, "en-us"
}
