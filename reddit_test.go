package reddit

import (
	"fmt"
	"log"
	"testing"

	"github.com/sheran/reddit/models"
)

func TestScavenge(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	creds, err := NewCredsFromTomlFile("../inchident/artfetch_creds.toml")
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

func TestUrl(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	creds, err := NewCredsFromTomlFile("../inchident/artfetch_creds.toml")
	if err != nil {
		panic(err)
	}

	reddit := NewReddit(creds)
	resp, err := reddit.PostUrl("/r/sneakpeekf1tests", "https://www.planetf1.com/news/adrian-newey-aston-martin-lawrence-stroll-contract-offer-rumour", "Rival team bid for Adrian Newey as big money contract offer emerges – report")
	if err != nil {
		t.Error(err)
	}
	fmt.Println(string(resp))
}

func TestDel(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	creds, err := NewCredsFromTomlFile("../inchident/artfetch_creds.toml")
	if err != nil {
		panic(err)
	}

	reddit := NewReddit(creds)
	resp, err := reddit.DelThing("t3_1bralib")
	if err != nil {
		t.Error(err)
	}
	fmt.Println(string(resp))
}
