all: cmd/takeown/takeown gnome/takeown.pyc gnome/takeown.pyo

BINDIR=/usr/local/bin
DATADIR=/usr/local/share
DESTDIR=

install: install-program install-kde install-gnome all

install-program: cmd/takeown/takeown
	mkdir -p $(DESTDIR)$(BINDIR)
	install -m 4755 cmd/takeown/takeown $(DESTDIR)$(BINDIR)

install-kde:
	mkdir -p $(DESTDIR)$(DATADIR)/kde4/services/ServiceMenus
	install -m 0644 kde/takeown.desktop $(DESTDIR)$(DATADIR)/kde4/services/ServiceMenus

install-gnome: gnome/takeown.pyc gnome/takeown.pyo
	mkdir -p $(DESTDIR)$(DATADIR)/nautilus-python/extensions
	install -m 0644 gnome/takeown.py* $(DESTDIR)$(DATADIR)/nautilus-python/extensions

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/takeown
	rm -f $(DESTDIR)$(DATADIR)/kde4/services/ServiceMenus/takeown.desktop
	rm -f $(DESTDIR)$(DATADIR)/nautilus-python/extensions/takeown.py*

clean:
	rm -f gnome/takeown.pyc gnome/takeown.pyo cmd/takeown/usage.go

cmd/takeown/usage.go: README.md build/gendoc.py
	python build/gendoc.py

cmd/takeown/takeown: cmd/takeown/usage.go cmd/takeown/*.go
	cd cmd/takeown && go build && cd ../..

gnome/takeown.pyc: gnome/takeown.py
	python -m compileall gnome/takeown.py

gnome/takeown.pyo: gnome/takeown.py
	python -O -m compileall gnome/takeown.py

test: cmd/takeown/takeown
	cd cmd/takeown && go test && cd ../..

gofmt:
	for f in cmd/takeown/*.go; do gofmt -w $$f; done

.PHONY: gofmt all install uninstall test install-program install-kde install-gnome
