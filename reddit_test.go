package reddit

import (
	"fmt"
	"log"
	"testing"

	"github.com/sheran/reddit/models"
)

func TestFetch(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	creds, err := NewCredsFromTomlFile("creds.toml")
	if err != nil {
		panic(err)
	}

	reddit := NewReddit(creds)
	post := &models.Post{
		Title:     "Prepping for github2 ",
		Body:      "This is body text",
		Subreddit: "sneakpeekf1tests",
		ApiType:   "json",
		Kind:      "self",
		Extension: "json",
	}

	body, err := reddit.PostForm(post)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(body))
}
