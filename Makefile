deb:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -o dist/systemd-cleaner .
	fpm -f -s dir -t deb -n systemd-cleaner -v 0.1.0 dist/systemd-cleaner=/usr/local/bin/ systemd-cleaner.service=/etc/systemd/system/