set -eu
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
for a in opus sonnet blank; do mkdir "$tmp/$a"; tar -C "$HOME/rashomon-eval/py27/$a/py2scan" --exclude .git -cf - . | tar -C "$tmp/$a" -xf -; done
cp "$HOME/rashomon-eval/steer/truth_builtin.py" "$tmp/"; cp "$HOME/rashomon-eval/steer/w3-xdiff/"*.json "$tmp/"
tar -C "$tmp" -cf - . | docker run -i --rm --network none -e PYTHONDONTWRITEBYTECODE=1 k-py27-inventory:eval sh -c '
mkdir /w && tar -C /w -xf - && chmod -R a+rX /w
echo "--- builtin ground truth (bytecode, scope-correct)"; python /w/truth_builtin.py /usr/local/lib/python2.7 /w
echo "--- determinism: --jobs 1 against --jobs 2 on the stdlib (md5 of JSON)"
for a in opus sonnet blank; do cd /w/$a; m1=$(python -m py2scan scan --format json --jobs 1 /usr/local/lib/python2.7 | md5sum | cut -c1-8); m2=$(python -m py2scan scan --format json --jobs 2 /usr/local/lib/python2.7 | md5sum | cut -c1-8); echo "$a jobs1=$m1 jobs2=$m2 $( [ $m1 = $m2 ] && echo same || echo DIFFERENT)"; done
echo "--- hostile inputs, run as an unprivileged user"
mkdir -p /h/c/loop && cd /h/c
printf "x = 1\000\n" > nul.py
python -c "print(\"x = \" + \"(\"*150 + \"1\" + \")\"*150)" > deep.py
printf "print 1\n" > unreadable.py; chmod 000 unreadable.py
ln -s missing.py broken.py; ln -s .. loop/up
printf "\357\273\277print 1\n" > bom.py
printf "print 1\r\nx = 2\r\n" > crlf.py
: > empty.py
printf "print 1\n" > "$(printf "caf\351.py")"
python -c "open(\"big.py\",\"w\").write(\"x = 1\n\"*200000)"
printf "# -*- coding: bogus -*-\nprint 1\n" > badcookie.py
chmod -R a+rX /h/c 2>/dev/null || true; chmod 000 /h/c/unreadable.py
for a in opus sonnet blank; do cd /w/$a; su nobody -s /bin/sh -c "python -m py2scan scan --format json /h/c > /tmp/$a.json 2>/tmp/$a.err; echo \$? > /tmp/$a.rc; python -m py2scan scan --html /tmp/$a.html /h/c >/dev/null 2>/tmp/$a.herr; echo \$? > /tmp/$a.hrc"
  printf "%s json_rc=%s html_rc=%s " $a $(cat /tmp/$a.rc) $(cat /tmp/$a.hrc)
  python -c "
import json,sys
try:
    d=json.load(open(\"/tmp/$a.json\")); print(\"files=%d rules=%s\" % (d[\"files\"], json.dumps(d[\"rules\"],sort_keys=True)))
except Exception as e: print(\"NO JSON: %r\" % e)"
  tail -c 300 /tmp/$a.err | tr "\n" " " | sed "s/^/   stderr: /"; echo; tail -c 200 /tmp/$a.herr | tr "\n" " " | sed "s/^/   html stderr: /"; echo
done'
