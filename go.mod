module mc-world-trimmer

go 1.20

require (
	github.com/Tnze/go-mc v1.18.3-0.20220528143224-a67d01b81f0d
	github.com/dustin/go-humanize v1.0.0
	github.com/klauspost/compress v1.16.5
	github.com/spf13/afero v1.8.2
)

require golang.org/x/text v0.3.4 // indirect

replace github.com/spf13/afero v1.8.2 => github.com/xtrafrancyz/afero v1.8.3-0.20220527135544-76d71183000e
