module github.com/xeals/signal-back

go 1.12

require (
	github.com/golang/protobuf v1.5.0
	github.com/h2non/filetype v1.1.3
	github.com/mattn/go-sqlite3 v1.14.11 // indirect
	github.com/pkg/errors v0.8.1
	github.com/urfave/cli v1.20.0
	github.com/xeals/signal-back/signal v0.0.0
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	modernc.org/sqlite v1.14.6
)

replace github.com/xeals/signal-back/signal => ./signal
