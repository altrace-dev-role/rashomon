set -eu
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
for a in opus sonnet blank; do mkdir "$tmp/$a"; tar -C "$HOME/rashomon-eval/py27/$a/py2scan" --exclude .git -cf - . | tar -C "$tmp/$a" -xf -; done
tar -C "$tmp" -cf - . | docker run -i --rm --network none -e PYTHONDONTWRITEBYTECODE=1 k-py27-inventory:eval sh -c '
mkdir /w && tar -C /w -xf - && chmod -R a+rX /w
mk() { d=/h/$1; mkdir -p $d; echo "print 1" > $d/ok.py; }
mk nul; printf "x = 1\000\n" > /h/nul/t.py
mk deep; python -c "print(\"x = \" + \"(\"*150 + \"1\" + \")\"*150)" > /h/deep/t.py
mk unreadable; printf "print 1\n" > /h/unreadable/t.py
mk broken; ln -s missing.py /h/broken/t.py
mk loop; mkdir /h/loop/d; ln -s .. /h/loop/d/up
mk bom; printf "\357\273\277print 1\n" > /h/bom/t.py
mk crlf; printf "print 1\r\nx = 2\r\n" > /h/crlf/t.py
mk empty; : > /h/empty/t.py
mk latin1name; printf "print 1\n" > "/h/latin1name/$(printf "caf\351.py")"
mk big; python -c "open(\"/h/big/t.py\",\"w\").write(\"x = 1\n\"*200000)"
mk badcookie; printf "# -*- coding: bogus -*-\nprint 1\n" > /h/badcookie/t.py
mk dirpy; mkdir /h/dirpy/pkg.py
chmod -R a+rX /h; chmod 000 /h/unreadable/t.py
printf "%-11s" case; for a in opus sonnet blank; do printf "| %-34s" $a; done; echo
for c in nul deep unreadable broken loop bom crlf empty latin1name big badcookie dirpy; do
  printf "%-11s" $c
  for a in opus sonnet blank; do
    cd /w/$a
    su nobody -s /bin/sh -c "python -m py2scan scan --format json /h/$c > /tmp/o.json 2> /tmp/e.txt; echo \$? > /tmp/rc" || true
    rc=$(cat /tmp/rc)
    res=$(python -c "
import json
try:
    d=json.load(open(\"/tmp/o.json\")); print(\"files=%d %s\" % (d[\"files\"], \",\".join(sorted(d[\"rules\"]))))
except Exception: print(\"no-report\")")
    tb=$(grep -c Traceback /tmp/e.txt || true)
    printf "| rc=%s %s%s" $rc "$res" "$( [ "$tb" != 0 ] && echo " TRACEBACK" )" | cut -c1-36 | tr -d "\n"; printf "%*s" $((36 - $(printf "| rc=%s %s%s" $rc "$res" "$( [ "$tb" != 0 ] && echo " TRACEBACK" )" | cut -c1-36 | wc -c))) ""
  done; echo
done'
