COLM = /home/thurston/pkgs/colm/bin/colm

all: build-binary

build-binary: everything

everything: everything.lm ../flux.lm $(COLM)
	$(COLM) -I.. -o $@ $<

clean: clean-binary

clean-binary:
	rm -f everything

.PHONY: all clean build-binary clean-binary

SED_SUBST = -e 's|@TOP_DIR@|/home/thurston/devel/flux/colm|g' -e 's|@BINARY_TARGET@|build-binary|g' -e 's|@INTERFACE_TARGET@|clean-interface|g' -e 's|@WITH_COLM@|/home/thurston/pkgs/colm|g' -e 's|@GO_INTERFACE@||g'

Makefile: Makefile.in /home/thurston/devel/flux/colm/.config.subst
	cd /home/thurston/devel/flux/colm && ./.config.subst

/home/thurston/devel/flux/colm/.config.subst: /home/thurston/devel/flux/colm/configure
	cd /home/thurston/devel/flux/colm && ./.config.reconf
