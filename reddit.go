package reddit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/sheran/reddit/models"
	"golang.org/x/time/rate"
)

type RateLimit struct {
	Remaining float64
	Reset     uint64
	Used      uint64
}

func NewRateLimit(hdr http.Header) *RateLimit {
	prefix := "X-Ratelimit-"
	ratelimit := &RateLimit{}
	for key, value := range hdr {
		if strings.HasPrefix(key, prefix) {
			if keyVal, found := strings.CutPrefix(key, prefix); found {
				switch keyVal {
				case "Used":
					val, err := strconv.ParseUint(value[0], 10, 64)
					if err != nil {
						continue
					}
					ratelimit.Used = val
				case "Reset":
					val, err := strconv.ParseUint(value[0], 10, 64)
					if err != nil {
						continue
					}
					ratelimit.Reset = val
				case "Remaining":
					flval, err := strconv.ParseFloat(value[0], 64)
					if err != nil {
						log.Println(err.Error())
						continue
					}
					ratelimit.Remaining = flval
				}
			}
		}
	}
	return ratelimit
}

func (rl *RateLimit) Wait() {
	if rl.Remaining <= 10 {
		log.Printf("[!!] No requests left, sleeping %d\n", rl.Reset)
		time.Sleep(time.Second * time.Duration(rl.Reset))
	}
}

func (rl *RateLimit) Limit() float64 {
	if rl.Reset == 0 {
		return 1
	}
	lim := rl.Remaining / float64(rl.Reset)
	return math.Floor(lim)
}

type Creds struct {
	Id     string `toml:"client_id"`
	Secret string `toml:"client_secret"`
	User   string `toml:"username"`
	Pass   string `toml:"password"`
	Agent  string `toml:"user_agent"`
}

func NewCredsFromTomlFile(credsFile string) (*Creds, error) {
	log.Printf("[+] loading creds file: %s\n", credsFile)
	var creds *Creds
	_, err := toml.DecodeFile(credsFile, &creds)
	if err != nil {
		return nil, err
	}
	return creds, nil
}

func (c *Creds) GetGrantJson() string {
	return fmt.Sprintf(`{"grant_type":"password","username":"%s","password":"%s"}`, c.User, c.Pass)
}

func readTokenFromFile() (string, error) {
	// Read token from file
	data, err := os.ReadFile("token.txt")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func getTokenFromReddit(creds *Creds) (string, error) {
	body := url.Values{}
	body.Set("grant_type", "password")
	body.Set("username", creds.User)
	body.Set("password", creds.Pass)
	client := &http.Client{}
	req, err := http.NewRequest("POST", "https://www.reddit.com/api/v1/access_token", strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Add("User-Agent", creds.Agent)
	req.SetBasicAuth(creds.Id, creds.Secret)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var responseJson map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseJson); err != nil {
		return "", err
	}
	if val, ok := responseJson["access_token"].(string); ok {
		return val, nil
	}
	return "", errors.New("no 'access_token' in response")
}

func getBearerToken(creds *Creds, forceReauth bool) (string, error) {
	if !forceReauth {
		token, err := readTokenFromFile()
		if err != nil {
			token, err = getTokenFromReddit(creds)
			if err != nil {
				return "", err
			}
			if err := os.WriteFile("token.txt", []byte(token), 0644); err != nil {
				return "", err
			}
		}
		return token, nil
	} else {
		token, err := getTokenFromReddit(creds)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile("token.txt", []byte(token), 0644); err != nil {
			return "", err
		}
		return token, nil
	}
}

type Reddit struct {
	token      string
	Client     *http.Client
	Limiter    *rate.Limiter
	ChanStream chan bool
	creds      *Creds
	exp        int
}

func NewReddit(creds *Creds) *Reddit {
	t, err := getBearerToken(creds, false)
	if err != nil {
		log.Fatalf("[!!] unable to create new reddit %s\n", err.Error())
	}
	return &Reddit{
		token:      t,
		Client:     &http.Client{},
		Limiter:    rate.NewLimiter(rate.Every(time.Second), 1),
		ChanStream: make(chan bool),
		creds:      creds,
		exp:        0,
	}
}

func (r *Reddit) StopStream() {
	r.ChanStream <- true
}

func (r *Reddit) CheckDups(sub, postTitle string) bool {
	fetchUrl := &url.URL{
		Host:     "oauth.reddit.com",
		Scheme:   "https",
		Path:     fmt.Sprintf("r/%s/new", sub),
		RawQuery: "limit=25",
	}
	listing, err := r.GetListing(fetchUrl)
	if err != nil {
		log.Println(err.Error())
		return true
	}
	for _, child := range listing.Data.Children {
		if strings.Trim(child.Data["title"].(string), " \n\t") == strings.Trim(postTitle, " \n\t") {
			return true
		}
	}

	return false
}

func (r *Reddit) StartStream(sub string, output chan *models.Listing) {
	log.Printf("[+] starting stream on subreddit: /r/%s\n", sub)
	go func() {
		oldData, err := r.GetLastPost(sub, "")
		if err != nil {
			log.Println(err.Error())
			return
		}
		for {
			data, err := r.GetLastPost(sub, "")
			if err != nil {
				log.Println(err.Error())
				continue
			}
			if data.GetFirstName() != "" && oldData.GetFirstName() != data.GetFirstName() {
				// check the dates
				delta := oldData.GetFirst().GetPublishTime().Sub(data.GetFirst().GetPublishTime())
				if delta < 0 {
					output <- data // I may have to check the date created also
					oldData = data
				}

			}
			select {
			case stop := <-r.ChanStream:
				if stop {
					return
				}
			default:

			}
		}
	}()
}

func (r *Reddit) GetListing(fetchUrl *url.URL) (*models.Listing, error) {
	var tell bool
	if err := r.Limiter.Wait(context.Background()); err != nil {
		sleep := int(math.Pow(float64(2), float64(r.exp)))
		log.Printf("[!] Rate Limit hit, sleeping %d secs\n", sleep)
		time.Sleep(time.Duration(sleep) * time.Second)
		if r.exp < (2 * 60) { // top out at 2 mintutes sleep
			r.exp += 1
		}
		tell = true
	}
	req, err := http.NewRequest("GET", fetchUrl.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("bearer %s", r.token))
	req.Header.Set("User-Agent", r.creds.Agent)

	resp, err := r.Client.Do(req)
	if err != nil {
		log.Printf("error in http request %d\n", resp.StatusCode)
		return nil, err
	}
	defer resp.Body.Close()
	rl := NewRateLimit(resp.Header)            // Check
	r.Limiter.SetLimit(rate.Limit(rl.Limit())) // and set Rate Limit
	// check and reset exp when we're at more or less full capacity
	if r.Limiter.Tokens() >= 1 {
		r.exp = 0
	}
	if tell {
		log.Printf("Reset: %d Used:%d Remain:%.2f Exp: %d Limit: %.2f/s Tokens:%.2f\n", rl.Reset, rl.Used, rl.Remaining, r.exp, rl.Limit(), r.Limiter.Tokens())
	}
	if resp.StatusCode == 401 {
		t, err := getBearerToken(r.creds, true)
		if err != nil {
			return nil, err
		}
		r.token = t
		listing, err := r.GetListing(fetchUrl)
		if err != nil {
			return nil, err
		}
		return listing, nil
	} else if resp.StatusCode != 200 {
		return nil, fmt.Errorf("non 200 status code: %d status: %s", resp.StatusCode, resp.Status)
	}
	return ReadJsonListing(resp.Body)
}

func (r *Reddit) PostForm(post *models.Post) ([]byte, error) {
	if !r.Limiter.Allow() {
		log.Println("[!] Rate Limit hit, sleeping 2 secs")
		time.Sleep(2 * time.Second)
	}
	postURL := "https://oauth.reddit.com/api/submit"
	postData := url.Values{
		"title":     {post.Title},
		"text":      {post.Body},
		"sr":        {post.Subreddit},
		"kind":      {post.Kind},
		"api_type":  {post.ApiType},
		"extension": {post.Extension},
	}
	if r.CheckDups(post.Subreddit, post.Title) {
		return nil, errors.New("post is a duplicate")
	}
	req, err := http.NewRequest("POST", postURL, strings.NewReader(postData.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "bearer "+r.token)
	req.Header.Set("User-Agent", r.creds.Agent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := r.Client.Do(req)
	if err != nil {
		log.Printf("error in http request %d\n", resp.StatusCode)
		return nil, err
	}
	rl := NewRateLimit(resp.Header)            // Check
	r.Limiter.SetLimit(rate.Limit(rl.Limit())) // and set Rate Limit
	if resp.StatusCode == 401 {
		_, err := getBearerToken(r.creds, true)
		if err != nil {
			log.Printf("error getting token: %s\n", err.Error())
			return nil, err
		}
		_, err = r.PostForm(post)
		if err != nil {
			log.Printf("error reposting form: %s\n", err.Error())
			return nil, err
		}
	} else if resp.StatusCode != 200 {
		log.Printf("non 200 error code when posting: %d\n", resp.StatusCode)
	} else {
		log.Println("postform is ok")
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return responseBody, nil

}

func (r *Reddit) GetLastPost(sub, after string) (*models.Listing, error) {
	fetchUrl := &url.URL{
		Host:     "oauth.reddit.com",
		Scheme:   "https",
		Path:     fmt.Sprintf("r/%s/new", sub),
		RawQuery: "limit=1",
	}
	data, err := r.GetListing(fetchUrl)
	if err != nil {
		return nil, err
	}
	return data, nil

}

func ReadJson(body io.ReadCloser) (map[string]interface{}, error) {
	jsonReader := json.NewDecoder(body)
	data := make(map[string]interface{}, 0)
	if err := jsonReader.Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

func ReadJsonListing(body io.ReadCloser) (*models.Listing, error) {
	jsonReader := json.NewDecoder(body)
	var data *models.Listing
	if err := jsonReader.Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}
