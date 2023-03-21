package reddit

import (
	"fmt"
	"log"
	"testing"
)

func TestScavenge(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	creds, err := NewCredsFromTomlFile("creds.toml")
	if err != nil {
		panic(err)
	}

	reddit := NewReddit(creds)
	fmt.Println(reddit.CheckDups("NonFatF1News", "Alonso gets Saudi GP F1 podium back after penalty overturned"))

}
