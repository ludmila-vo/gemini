all:
	go install
	cd ~/w/proxy; gemini --include main.go -p 'create test to call cooking.voilokov.com and check that error is 502'
