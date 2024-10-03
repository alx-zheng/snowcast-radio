all:
	go build -buildvcs=false ./cmd/snowcast_control
	go build -buildvcs=false ./cmd/snowcast_server
	go build -buildvcs=false ./cmd/snowcast_listener
clean:
	rm -fv snowcast_control snowcast_server snowcast_listener