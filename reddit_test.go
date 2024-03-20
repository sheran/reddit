package reddit

import (
	"fmt"
	"log"
	"testing"

	"github.com/sheran/reddit/models"
)

func TestScavenge(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	creds, err := NewCredsFromTomlFile("../reddit-stream/creds/sgnewsbot_creds.toml")
	if err != nil {
		panic(err)
	}

	reddit := NewReddit(creds)
	ch := make(chan *models.Listing)

	reddit.StartStream("sneakpeekf1tests", ch)

	for data := range ch {
		fmt.Println(data.GetFirst().GetURL())
	}

}
