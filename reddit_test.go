package reddit

import (
	"fmt"
	"log"
	"testing"
)

func TestScavenge(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	creds, err := NewCredsFromTomlFile("../reddit-stream/creds/f1newsbot_creds.toml")
	if err != nil {
		panic(err)
	}

	reddit := NewReddit(creds)
	for i := 0; i < 5; i++ {
		fmt.Println(reddit.CheckDups("SingaporeRaw", "Transport Minister S Iswaran assisting in CPIB investigation"))
	}

}
