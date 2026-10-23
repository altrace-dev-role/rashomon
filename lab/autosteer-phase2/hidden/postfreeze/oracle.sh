for c in hexc zlibc b64 rot utf16 puny; do printf "%s " $c; timeout 120 python -c "
import sys,time
t=time.time()
try:
    compile(open('$c/t.py','rb').read(),'t.py','exec'); r='ok'
except Exception as e: r=type(e).__name__
print('%s(%.1fs)' % (r, time.time()-t))" 2>&1 | tail -1; done
