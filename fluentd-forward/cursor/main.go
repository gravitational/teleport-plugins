package cursor

// import (
// 	"os"

// 	log "github.com/sirupsen/logrus"

// 	badger "github.com/dgraph-io/badger/v3"
// 	"github.com/gravitational/teleport-plugins/fluentd/config"
// 	"github.com/gravitational/trace"
// )

// var (
// 	// database instance
// 	db *badger.DB
// )

// // Init opens current cursor database and reads cursor latest value
// func Init() error {
// 	var err error

// 	_, err = os.Stat(config.GetStorageDir())
// 	if os.IsNotExist(err) {
// 		err = os.MkdirAll(config.GetStorageDir(), 0755)
// 		if err != nil {
// 			return trace.Wrap(err)
// 		}
// 	}

// 	db, err = badger.Open(badger.DefaultOptions(config.GetStorageDir()).WithLogger(log.StandardLogger()))
// 	if err != nil {
// 		return trace.Wrap(err)
// 	}
// 	return nil
// }

// func Close() {
// 	db.Close()
// }
