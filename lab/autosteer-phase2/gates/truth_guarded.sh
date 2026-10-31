set -eu
xd="$HOME/rashomon-eval/steer/w3-xdiff"
tar -C "$xd" -cf - opus.json sonnet.json blank.json truth.json -C "$HOME/rashomon-eval/steer" truth27.py | docker run -i --rm --network none k-py27-inventory:eval sh -c 'mkdir /x && tar -C /x -xf - && python /x/truth27.py /usr/local/lib/python2.7 /x && cd /usr/local/lib/python2.7 && grep -n -E "(^|[^a-zA-Z_])[uU][rR][\"\x27]" json/__init__.py posixpath.py lib2to3/fixes/fix_unicode.py | head -5'
