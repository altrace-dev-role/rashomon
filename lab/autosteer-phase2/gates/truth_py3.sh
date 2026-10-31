set -eu
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
docker run --rm --network none k-py27-inventory:eval sh -c 'cd /usr/local/lib/python2.7 && find . -name "*.py" | tar -cf - -T -' | tar -C "$tmp" -xf -
python3 --version
python3 "$HOME/rashomon-eval/steer/truth.py" "$tmp" "$HOME/rashomon-eval/steer/w3-xdiff"
