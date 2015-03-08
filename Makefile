#
# makefile for packaging.
#

prefix=/usr/sbin
bindir=$(prefix)

all: song_tracker2

install:
	@install -D -m 0755 song_tracker2 $(DESTDIR)$(bindir)/song_tracker2
	@install -D -m 0644 song_tracker2.init $(DESTDIR)/etc/init.d/song_tracker2
	@install -D -m 0755 song_tracker2.conf.example $(DESTDIR)/etc/song_tracker2.conf
