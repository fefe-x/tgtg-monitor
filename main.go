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
	} `json:"item,omitempty"`

	DisplayName    string `json:"display_name"`
	PickupInterval struct {
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
	} `json:"pickup_interval,omitempty"`
	ItemsAvailable int `json:"items_available"`
}

type Account struct {
	Email string `json:"email"`
	//PollingId      string `json:"polling-id"`
	AccessToken    string `json:"access-token"`
	RefreshToken   string `json:"refresh-token"`
	DatadomeCookie string `json:"datadome"`
	UserId         string `json:"user-id"`
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
		FillerType string `json:"filler_type,omitempty"`
	} `json:"bucket,omitempty"`
	Origin struct {
		Latitude  float64 `json:"latitude,omitempty"`
		Longitude float64 `json:"longitude,omitempty"`
	} `json:"origin,omitempty"`
	Paging struct {
		Page int `json:"page,omitempty"`
		Size int `json:"size,omitempty"`
	} `json:"paging,omitempty"`
	Radius float64 `json:"radius,omitempty"`
	UserId string  `json:"user_id,omitempty"`
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
			} `json:"item,omitempty"`

			DisplayName    string `json:"display_name"`
			PickupInterval struct {
				Start time.Time `json:"start"`
				End   time.Time `json:"end"`
			} `json:"pickup_interval,omitempty"`
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

//const webhook_url string = os.Getenv(key)

func login(client *http.Client, account Account) {
	fmt.Println("logging in...")

	loginPayload := LoginPayload{DeviceType: "ANDROID", Email: account.Email}

	loginPayloadMarshalled, err := json.Marshal(loginPayload)
	loginReq, err := http.NewRequest("POST", login_url, bytes.NewReader(loginPayloadMarshalled))
	check(err)
	loginReq.Header = http.Header{
		"user-agent":      {"TGTG/23.8.11 Dalvik/2.1.0 (Linux; U; Android 11; GM1913 Build/RKQ1.201022.002)"},
		"accept-language": {"en-GB"},
		"accept":          {"application/json"},
		"Content-Type":    {"application/json; charset=utf-8"},
		//"Content-Length":  {"55"},
		"accept-encoding": {"gzip"},
	}

	loginResp, err := client.Do(loginReq)
	check(err)
	if status := loginResp.StatusCode; status == 429 || status == 403 {
		panic(status)
	}
	loginBody, err := io.ReadAll(loginResp.Body)
	check(err)
	loginResp.Body.Close()

	var result LoginResponse
	if err := json.Unmarshal(loginBody, &result); err != nil {
		panic(err)
	}
	pollingPayload := PollingPayload{DeviceType: "ANDROID", Email: account.Email, RequestPollingId: result.PollingID}
	pollingPayloadMarshalled, err := json.Marshal(pollingPayload)
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
			//"Content-Length":  {"145"},
			"accept-encoding": {"gzip"},
		}

		pollingResp, err := client.Do(pollingRequest)
		fmt.Println("polling status", pollingResp.StatusCode)
		check(err)

		pollingBody, err := io.ReadAll(pollingResp.Body)
		if len(pollingBody) == 0 {
			fmt.Println("empty polling body")
			continue
		} else {
			waiting = false
		}
		//fmt.Println("body:", pollingBody)
		check(err)

		pollingResp.Body.Close()
		if err := json.Unmarshal(pollingBody, &pollingResult); err == io.EOF {
			panic(err)
		}
	}

	account.AccessToken = pollingResult.AccessToken
	account.RefreshToken = pollingResult.RefreshToken

	// request to startup_url

	var startupResult StartupResponse

	startupRequest, err := http.NewRequest("POST", favorites_url, nil)
	check(err)
	startupRequest.Header = http.Header{
		"user-agent":      {"TGTG/23.8.11 Dalvik/2.1.0 (Linux; U; Android 11; GM1913 Build/RKQ1.201022.002)"},
		"accept-language": {"en-GB"},
		"accept":          {"application/json"},
		"authorization":   {"Bearer " + account.AccessToken},
		"Content-Type":    {"application/json; charset=utf-8"},
		//"Content-Length":  {"145"},
		"accept-encoding": {"gzip"},
	}

	startupResp, err := client.Do(startupRequest)

	check(err)

	startupBody, err := io.ReadAll(startupResp.Body)

	check(err)

	startupResp.Body.Close()
	if err := json.Unmarshal(startupBody, &startupResult); err != nil {
		panic(err)
	}

	account.UserId = startupResult.User.UserId

	fmt.Println("logged in")

}

func getFavorites(client *http.Client, account Account) FavoritesResponse {
	// send request to item endpoint, return struct that contains relevant information to be sent in webhook/used to retry
	// item name, stock level, ...
	var result FavoritesResponse
	favoritesPayload := FavoritesPayload{
		Bucket: struct {
			FillerType string "json:\"filler_type,omitempty\""
		}{
			FillerType: "Favorites",
		},
		Origin: struct {
			Latitude  float64 "json:\"latitude,omitempty\""
			Longitude float64 "json:\"longitude,omitempty\""
		}{
			Latitude:  0.0,
			Longitude: 0.0,
		},
		Paging: struct {
			Page int "json:\"page,omitempty\""
			Size int "json:\"size,omitempty\""
		}{
			Page: 0,
			Size: 50,
		},
		Radius: 0,
		UserId: account.UserId,
	}

	favoritesPayloadMarshalled, err := json.Marshal(favoritesPayload)
	favoritesRequest, err := http.NewRequest("POST", favorites_url, bytes.NewReader(favoritesPayloadMarshalled))
	check(err)

	favoritesRequest.Header = http.Header{
		"user-agent":      {"TGTG/23.8.11 Dalvik/2.1.0 (Linux; U; Android 11; GM1913 Build/RKQ1.201022.002)"},
		"accept-language": {"en-GB"},
		"accept":          {"application/json"},
		"authorization":   {"Bearer " + account.AccessToken},
		"Content-Type":    {"application/json; charset=utf-8"},
		//"Content-Length":  {"145"},
		"accept-encoding": {"gzip"},
	}

	favoritesResp, err := client.Do(favoritesRequest)

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

func getRestockedItems(client *http.Client, account Account, new FavoritesResponse) bool {
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
						Value: fmt.Sprint(item.Item.ItemPrice.MinorUnits/100.0) + " " + item.Item.ItemPrice.Code,
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

	account := Account{
		Email:          scanned[0],
		AccessToken:    "",
		RefreshToken:   "",
		DatadomeCookie: "",
		UserId:         "",
		Favorites:      make(map[string]Favorite),
		WebhookUrl:     scanned[1],
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
			for i := 0; i < len(favorites.Bucket.Items); i++ {
				current := favorites.Bucket.Items[i]
				account.Favorites[current.Item.ItemID] = current
			}
			account.FavoritesSet = true
		} else { // check if stock was 0 and is now >= 1 for any favorite, send webhook if yes
			wasRestocked := getRestockedItems(client, account, favorites)
			if wasRestocked {
				fmt.Println("found a restock")
			}
		}
	}

}
