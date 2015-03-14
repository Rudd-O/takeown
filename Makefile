all: cmd/takeown/takeown

BINDIR=/usr/local/bin
DATADIR=/usr/local/share
DESTDIR=

install: all
	mkdir -p $(DESTDIR)$(BINDIR)
	mkdir -p $(DESTDIR)$(DATADIR)/kde4/services/ServiceMenus
	install -m 4755 cmd/takeown/takeown $(DESTDIR)$(BINDIR)
	install -m 0644 kde/takeown.desktop $(DESTDIR)$(DATADIR)/kde4/services/ServiceMenus

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/takeown
	rm -f $(DESTDIR)$(DATADIR)/kde4/services/ServiceMenus/takeown.desktop

cmd/takeown/usage.go: README.md build/gendoc.py
	python build/gendoc.py

cmd/takeown/takeown: cmd/takeown/usage.go cmd/takeown/*.go
	cd cmd/takeown && go build && cd ../..

gofmt:
	for f in cmd/takeown/*.go; do echo gofmt -w $$f; done

.PHONY: gofmt all install uninstall
