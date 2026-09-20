PREFIX ?= /usr/local

install:
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 755 shineyshot $(DESTDIR)$(PREFIX)/bin/
	install -d $(DESTDIR)$(PREFIX)/share/applications
	install -m 644 assets/shineyshot.desktop $(DESTDIR)$(PREFIX)/share/applications/
	install -d $(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps
	install -m 644 assets/icons/shineyshot.svg $(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps/
	install -d $(DESTDIR)$(PREFIX)/share/man/man1
	install -m 644 docs/shineyshot.1 $(DESTDIR)$(PREFIX)/share/man/man1/

.PHONY: install
