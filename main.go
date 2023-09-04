package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"
)

type Favorite struct {
	Item struct {
		ItemID    string `json:"item_id"`
		ItemPrice struct {
			Code       string `json:"code"`
			MinorUnits int    `json:"minor_units"`
			Decimals   int    `json:"decimals"`
		} `json:"item_price"`
	} `json:"item"`

	DisplayName    string `json:"display_name"`
	PickupInterval struct {
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
	} `json:"pickup_interval"`
	ItemsAvailable int `json:"items_available"`
}

type Account struct {
	Email string `json:"email"`
	//PollingId      string `json:"polling-id"`
	AccessToken    string      `json:"access-token"`
	RefreshToken   string      `json:"refresh-token"`
	DatadomeCookie http.Cookie `json:"datadome"`
	UserId         string      `json:"user-id"`
	Favorites      map[string]Favorite
	FavoritesSet   bool
	WebhookUrl     string
}

type LoginResponse struct {
	PollingID string `json:"polling_id"`
	State     string `json:"state"`
}

type PollingResponse struct {
	AccessToken    string `json:"access_token"`
	AccessTokenTTL string `json:"access_token_ttl_seconds"`
	RefreshToken   string `json:"refresh_token"`
}

type LoginPayload struct {
	DeviceType string `json:"device_type" default:"ANDROID"`
	Email      string `json:"email"`
}

type PollingPayload struct {
	DeviceType       string `json:"device_type" default:"ANDROID"`
	Email            string `json:"email"`
	RequestPollingId string `json:"request_polling_id"`
}

type FavoritesPayload struct {
	Bucket struct {
		FillerType string `json:"filler_type"`
	} `json:"bucket"`
	Origin struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"origin"`
	Paging struct {
		Page int `json:"page"`
		Size int `json:"size"`
	} `json:"paging"`
	Radius float64 `json:"radius"`
	UserId string  `json:"user_id"`
}

type FavoritesResponse struct {
	Bucket struct {
		Items []struct {
			Item struct {
				ItemID    string `json:"item_id"`
				ItemPrice struct {
					Code       string `json:"code"`
					MinorUnits int    `json:"minor_units"`
					Decimals   int    `json:"decimals"`
				} `json:"item_price"`
			} `json:"item"`

			DisplayName    string `json:"display_name"`
			PickupInterval struct {
				Start time.Time `json:"start"`
				End   time.Time `json:"end"`
			} `json:"pickup_interval"`
			ItemsAvailable int `json:"items_available"`
		} `json:"items"`
	} `json:"mobile_bucket"`
}

type StartupResponse struct {
	User struct {
		UserId string `json:"user_id"`
	} `json:"user"`
}

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Embed struct {
	Title  string  `json:"title"`
	Color  int     `json:"color"`
	Fields []Field `json:"fields"`
}

type EmbedPayload struct {
	Embeds    []Embed `json:"embeds"`
	Username  string  `json:"username"`
	AvatarURL string  `json:"avatar_url"`
}

const login_url string = "https://apptoogoodtogo.com/api/auth/v4/authByEmail"

const polling_url string = "https://apptoogoodtogo.com/api/auth/v4/authByRequestPollingId"

const favorites_url string = "https://apptoogoodtogo.com/api/discover/v1/bucket"

const startup_url string = "https://apptoogoodtogo.com/api/app/v1/onStartup"

func login(client *http.Client, account *Account) {
	fmt.Println("logging in...")

	loginPayload := LoginPayload{DeviceType: "ANDROID", Email: account.Email}

	loginPayloadMarshalled, err := json.Marshal(loginPayload)
	check(err)
	loginReq, err := http.NewRequest("POST", login_url, bytes.NewReader(loginPayloadMarshalled))
	check(err)
	loginReq.Header = http.Header{
		"user-agent":      {"TGTG/23.8.11 Dalvik/2.1.0 (Linux; U; Android 11; GM1913 Build/RKQ1.201022.002)"},
		"accept-language": {"en-GB"},
		"accept":          {"application/json"},
		"Content-Type":    {"application/json; charset=utf-8"},
		"accept-encoding": {"gzip"},
	}

	loginResp, err := client.Do(loginReq)
	check(err)
	if status := loginResp.StatusCode; status == 429 || status == 403 {
		panic(status)
	}
	updateDatadome(*loginResp, account)
	loginBody, err := io.ReadAll(loginResp.Body)
	check(err)
	loginResp.Body.Close()

	var result LoginResponse
	if err := json.Unmarshal(loginBody, &result); err != nil {
		panic(err)
	}
	pollingPayload := PollingPayload{DeviceType: "ANDROID", Email: account.Email, RequestPollingId: result.PollingID}
	pollingPayloadMarshalled, err := json.Marshal(pollingPayload)
	check(err)
	waiting := true
	var pollingResult PollingResponse

	for waiting {
		time.Sleep(5 * time.Second)
		pollingRequest, err := http.NewRequest("POST", polling_url, bytes.NewReader(pollingPayloadMarshalled))
		check(err)

		pollingRequest.Header = http.Header{
			"user-agent":      {"TGTG/23.8.11 Dalvik/2.1.0 (Linux; U; Android 11; GM1913 Build/RKQ1.201022.002)"},
			"accept-language": {"en-GB"},
			"accept":          {"application/json"},
			"Content-Type":    {"application/json; charset=utf-8"},
			"accept-encoding": {"gzip"},
		}

		pollingResp, err := client.Do(pollingRequest)
		fmt.Println("polling status", pollingResp.StatusCode)
		check(err)
		updateDatadome(*pollingResp, account)
		pollingBody, err := io.ReadAll(pollingResp.Body)
		if len(pollingBody) == 0 {
			fmt.Println("empty polling body")
			continue
		} else {
			waiting = false
		}
		check(err)

		pollingResp.Body.Close()
		if err := json.Unmarshal(pollingBody, &pollingResult); err == io.EOF {
			panic(err)
		}
	}
	account.AccessToken = pollingResult.AccessToken
	account.RefreshToken = pollingResult.RefreshToken
	var startupResult StartupResponse

	startupRequest, err := http.NewRequest("POST", startup_url, nil)
	check(err)
	startupRequest.Header = http.Header{
		"user-agent":      {"TGTG/23.8.11 Dalvik/2.1.0 (Linux; U; Android 11; GM1913 Build/RKQ1.201022.002)"},
		"accept-language": {"en-GB"},
		"accept":          {"application/json"},
		"authorization":   {"Bearer " + account.AccessToken},
		"Content-Type":    {"application/json; charset=utf-8"},
		"accept-encoding": {"gzip"},
	}

	startupResp, err := client.Do(startupRequest)

	check(err)
	updateDatadome(*startupResp, account)
	startupBody, err := io.ReadAll(startupResp.Body)

	check(err)

	startupResp.Body.Close()
	if err := json.Unmarshal(startupBody, &startupResult); err != nil {
		panic(err)
	}
	account.UserId = startupResult.User.UserId

	fmt.Println("logged in with user-id", account.UserId)
}

func updateDatadome(resp http.Response, account *Account) {
	cookie := resp.Header.Get("set-cookie")

	var pairs = strings.Split(cookie, ";")
	account.DatadomeCookie = http.Cookie{
		Name:       "datadome",
		Value:      pairs[0],
		Path:       "/",
		Domain:     ".apptoogoodtogo.com",
		Expires:    time.Time{},
		RawExpires: "",
		MaxAge:     0,
		Secure:     true,
		HttpOnly:   false,
		SameSite:   0,
		Raw:        "",
		Unparsed:   []string{},
	}

}

func getFavorites(client *http.Client, account *Account) FavoritesResponse {
	// send request to item endpoint, return struct that contains relevant information to be sent in webhook/used to retry
	// item name, stock level, ...
	var result FavoritesResponse
	favoritesPayload := FavoritesPayload{
		Bucket: struct {
			FillerType string `json:"filler_type"`
		}{
			FillerType: "Favorites",
		},
		Origin: struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}{
			Latitude:  0.0,
			Longitude: 0.0,
		},
		Paging: struct {
			Page int `json:"page"`
			Size int `json:"size"`
		}{
			Page: 0,
			Size: 50,
		},
		Radius: 30.0,
		UserId: account.UserId,
	}

	favoritesPayloadMarshalled, err := json.Marshal(favoritesPayload)
	check(err)
	favoritesRequest, err := http.NewRequest("POST", favorites_url, bytes.NewReader(favoritesPayloadMarshalled))
	check(err)

	var bearer = "Bearer " + account.AccessToken
	var datadomeCookie = account.DatadomeCookie

	favoritesRequest.Header = http.Header{
		"user-agent":      {"TGTG/23.8.11 Dalvik/2.1.0 (Linux; U; Android 11; GM1913 Build/RKQ1.201022.002)"},
		"accept-language": {"en-GB"},
		"accept":          {"application/json"},
		"Content-Type":    {"application/json; charset=utf-8"},
		"accept-encoding": {"gzip"},
	}

	favoritesRequest.Header.Add("Authorization", bearer)
	favoritesRequest.Header.Add("cookie", datadomeCookie.Value)

	favoritesResp, err := client.Do(favoritesRequest)
	for favoritesResp.StatusCode == 403 || favoritesResp.StatusCode == 429 {
		fmt.Println(favoritesResp.StatusCode)
		time.Sleep(1000 * time.Second)
	}
	updateDatadome(*favoritesResp, account)
	check(err)
	favoritesBody, err := io.ReadAll(favoritesResp.Body)

	check(err)
	favoritesResp.Body.Close()
	if err := json.Unmarshal(favoritesBody, &result); err != nil {
		panic(err)
	}

	return result

}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func getRestockedItems(client *http.Client, account *Account, new FavoritesResponse) bool {
	wasRestock := false
	newItems := new.Bucket.Items
	for i := 0; i < len(newItems); i++ {
		currentNew := newItems[i]
		currentOld := account.Favorites[currentNew.Item.ItemID]
		if currentOld.ItemsAvailable < currentNew.ItemsAvailable {
			wasRestock = true
			sendEmbed(client, account.WebhookUrl, currentNew)
			currentOld.ItemsAvailable = currentNew.ItemsAvailable
		}
	}
	return wasRestock
}

func sendEmbed(client *http.Client, webhookUrl string, item Favorite) {
	var embed EmbedPayload = EmbedPayload{
		Embeds: []Embed{
			{
				Title: "TGTG restock found!",
				Color: 55144,
				Fields: []Field{

					{
						Name:  "Product",
						Value: item.DisplayName,
					},
					{
						Name:  "Duration",
						Value: item.PickupInterval.Start.String() + " - " + item.PickupInterval.End.String(),
					},
					{
						Name:  "Price",
						Value: fmt.Sprint(float64(item.Item.ItemPrice.MinorUnits)/100.0) + " " + item.Item.ItemPrice.Code,
					},
				},
			},
		},
		Username:  "to good to go-monitor",
		AvatarURL: "https://de.wikipedia.org/wiki/Too_Good_To_Go#/media/Datei:Too_Good_To_Go_Logo.svg",
	}
	embedMarshalled, err := json.Marshal(embed)
	check(err)
	embedRequest, err := http.NewRequest("POST", webhookUrl, bytes.NewReader(embedMarshalled))
	check(err)
	embedRequest.Header = http.Header{
		"user-agent":      {"TGTG/23.8.11 Dalvik/2.1.0 (Linux; U; Android 11; GM1913 Build/RKQ1.201022.002)"},
		"accept-language": {"en-GB"},
		"accept":          {"application/json"},
		"Content-Type":    {"application/json; charset=utf-8"},
		"accept-encoding": {"gzip"},
	}
	embedResponse, err := client.Do(embedRequest)
	check(err)
	if embedResponse.StatusCode == 204 {
		fmt.Println("sent webhhook")
	} else {
		fmt.Println(embedResponse.Body)
	}
}

func main() {
	dat, err := os.Open("./config")
	check(err)
	defer dat.Close()

	var scanned [2]string

	scanner := bufio.NewScanner(dat)

	for i := 0; i < 2; i++ {
		scanner.Scan()
		scanned[i] = scanner.Text()
	}

	account := &Account{
		Email:          scanned[0],
		AccessToken:    "",
		RefreshToken:   "",
		DatadomeCookie: http.Cookie{},
		UserId:         "",
		Favorites:      make(map[string]Favorite),
		WebhookUrl:     scanned[1],
		FavoritesSet:   false,
	}
	jar, err := cookiejar.New(nil)
	check(err)

	client := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
	}
	login(client, account)
	for {
		time.Sleep(10 * time.Second)
		fmt.Println("Monitoring...")
		favorites := getFavorites(client, account)
		if !account.FavoritesSet { // initialise favorites if not loaded yet
			fmt.Println("initialising favorites")
			for i := 0; i < len(favorites.Bucket.Items); i++ {
				current := favorites.Bucket.Items[i]
				account.Favorites[current.Item.ItemID] = current
				if i == 0 { // send one webhook as test
					sendEmbed(client, account.WebhookUrl, account.Favorites[current.Item.ItemID])
				}
			}
			account.FavoritesSet = true
		} else { // loop through favorites, sending webhook if old stock level < new stock level
			wasRestocked := getRestockedItems(client, account, favorites)
			if wasRestocked {
				fmt.Println("found a restock")
			}
		}
	}

}
