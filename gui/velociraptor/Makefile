all:
	echo "module fake_nodejs_module // Exclude this directory from Go tools\n\ngo 1.16" > 'node_modules/go.mod'
	npm run start

build: FORCE
	echo "module fake_nodejs_module // Exclude this directory from Go tools\n\ngo 1.16" > 'node_modules/go.mod'
	npm run build
	echo > build/static/.keep

FORCE: ;
