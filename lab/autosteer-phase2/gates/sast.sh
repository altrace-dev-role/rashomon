set -eu
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
for a in opus sonnet blank; do mkdir "$tmp/$a"; tar -C "$HOME/rashomon-eval/py27/$a/py2scan" --exclude .git -cf - py2scan | tar -C "$tmp/$a" -xf -; done
mkdir "$tmp/control"; printf 'import yaml, subprocess\ndef f(x):\n    subprocess.call(x, shell=True)\n    return yaml.load(x)\n' > "$tmp/control/bad.py"
tar -C "$tmp" -cf - opus sonnet blank control | docker run -i --rm k-py27-inventory:eval sh -c '
mkdir /arms && tar -C /arms -xf - && pip install -q --disable-pip-version-check "bandit==1.6.2" 2>&1 | tail -2; bandit --version 2>&1 | head -1
for a in control opus sonnet blank; do echo "== $a"; bandit -r /arms/$a -f custom --msg-template "{relpath}:{line} {test_id} {severity}/{confidence} {msg}" 2>&1 | sed "s|/arms/$a/||" | grep -v -E "^\[main\]|^$" ; echo "exit=$?"; done'
