.PHONY: release

release:
ifndef VERSION
	$(error VERSION is required: make release VERSION=x.y.z)
endif
	go test -race ./...
	go vet ./...
	git tag v$(VERSION)
	git push origin v$(VERSION)
	gh release create v$(VERSION) --generate-notes
