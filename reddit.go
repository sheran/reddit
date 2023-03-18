package reddit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/sheran/reddit/models"
	"golang.org/x/time/rate"
)

type Creds struct {
	Id     string `toml:"client_id"`
	Secret string `toml:"client_secret"`
	User   string `toml:"username"`
	Pass   string `toml:"password"`
	Agent  string `toml:"user_agent"`
}

func NewCredsFromTomlFile(credsFile string) (*Creds, error) {
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
	data, err := ioutil.ReadFile("token.txt")
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
			if err := ioutil.WriteFile("token.txt", []byte(token), 0644); err != nil {
				return "", err
			}
		}
		return token, nil
	} else {
		token, err := getTokenFromReddit(creds)
		if err != nil {
			return "", err
		}
		if err := ioutil.WriteFile("token.txt", []byte(token), 0644); err != nil {
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
}

func NewReddit(creds *Creds) *Reddit {
	t, err := getBearerToken(creds, false)
	if err != nil {
		log.Fatalf("unable to create new reddit %s\n", err.Error())
	}
	return &Reddit{
		token:      t,
		Client:     &http.Client{},
		Limiter:    rate.NewLimiter(rate.Every(2*time.Second), 1),
		ChanStream: make(chan bool),
		creds:      creds,
	}
}

func (r *Reddit) StopStream() {
	r.ChanStream <- true
}

func (r *Reddit) StartStream(sub string, output chan *models.Listing) {
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
	err := r.Limiter.Wait(context.Background())
	if err != nil {
		log.Println(err.Error())
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
		return nil, fmt.Errorf("non 200 status code: %d", resp.StatusCode)
	}
	return ReadJsonListing(resp.Body)
}

func (r *Reddit) PostText(post *models.Post) ([]byte, error) {
	jsonPostData, err := json.Marshal(post)
	if err != nil {
		return nil, err
	}
	postUrl := "https://oauth.reddit.com/api/submit"
	req, err := http.NewRequest("POST", postUrl, bytes.NewBuffer(jsonPostData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("bearer %s", r.token))
	req.Header.Set("User-Agent", r.creds.Agent)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return responseBody, nil

}

func (r *Reddit) PostForm(post *models.Post) ([]byte, error) {
	postURL := "https://oauth.reddit.com/api/submit"
	postData := url.Values{
		"title":     {post.Title},
		"text":      {post.Body},
		"sr":        {post.Subreddit},
		"kind":      {post.Kind},
		"api_type":  {post.ApiType},
		"extension": {post.Extension},
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
	responseBody, err := ioutil.ReadAll(resp.Body)
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
