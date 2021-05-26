package main

import (
	"github.com/gravitational/trace"
)

// Poll represents periodical event poll
type Poll struct {
	// fluentd is an instance of Fluentd client
	fluentd *FluentdClient

	// teleport is an instance of Teleport client
	teleport *TeleportClient

	// cursor is an instance of cursor manager
	cursor *Cursor
}

// NewPoll builds new Poll structure
func NewPoll(c *Config) (*Poll, error) {
	k, err := NewCursor(c)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	f, err := NewFluentdClient(c)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	cursor, err := k.Get()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	t, err := NewTeleportClient(c, cursor)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &Poll{fluentd: f, teleport: t, cursor: k}, nil
}

// Close closes all connections
func (p *Poll) Close() {
	p.teleport.Close()
}

func (p *Poll) Run() {
	// // v, _ := k.Get()
	// // logrus.Printf(v)
	// // k.Set("")

	// err = f.Send(dummy{A: "1", B: "2"})
	// if err != nil {
	// 	log.Error(trace.DebugReport(err))
	// 	os.Exit(-1)
	// }

	//t.Test()
	// e, err := t.Next()
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// for e != nil {
	// 	e, err := t.Next()
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	if e != nil {
	// 		fmt.Println(e.GetID())
	// 	} else {
	// 		break
	// 	}
	// }

	// err = fluentd.Init()
	// if err != nil {
	// 	log.Fatal(trace.DebugReport(err))
	// 	os.Exit(-1)
	// }

}
