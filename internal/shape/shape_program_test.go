package shape

import (
	"encoding/json"
	"testing"
)

// TestProgramIsAProgram: the program field names a program, or is null.
//
// Measured on a real session before this held: three of twenty-six
// declarations recorded `&&`, `(` or nothing as the program -- about one in
// eight of the single field that says what ran. A value that cannot be a
// program is worse than no value, because null already means "we could not
// tell" and reads that way, while `&&` reads as a fact.
func TestProgramIsAProgram(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
		want string // "" means the program must be null
		why  string
	}{
		{name: "subshell", cmd: "( cd /tmp && ls )", want: "ls",
			why: "recorded `(` before the program was found, and `cd` before it looked past a directory change"},
		{name: "brace group", cmd: "{ ls; }", want: "ls",
			why: "`{` is a shell keyword the tokenizer does not mark as a metacharacter"},
		{name: "leading operator", cmd: "&& go build", want: "go"},
		{name: "leading pipe", cmd: "| grep x", want: "grep"},
		{name: "leading semicolon", cmd: "; ls", want: "ls"},
		{name: "redirect first", cmd: "> out.txt ls", want: "ls",
			why: "the filename after a redirect is not the program"},

		// Operators the tokenizer emits as TWO tokens, because it doubles a
		// metacharacter only when the next byte is the same byte. Skipping
		// "the operator and one token" here skips the operator's second half
		// and keeps the filename -- which recorded customer-list.csv as the
		// program of the first line below. A regression against the
		// behaviour before this function, which recorded `>`: useless, but no
		// content.
		{name: "redirect >&", cmd: ">& /home/alice/customer-list.csv ls", want: "ls"},
		{name: "redirect >|", cmd: ">| /tmp/acme-merger-notes.txt ls", want: "ls"},
		{name: "redirect <>", cmd: "<> /var/db/prod.sqlite ls", want: "ls"},
		{name: "here-string", cmd: `<<< "hunter2-password" cat`, want: "cat",
			why: "a here-string's word is literal text, not even a filename"},
		{name: "fd duplication", cmd: ">&2 echo hi", want: "echo"},
		{name: "input fd duplication", cmd: "<&3 read x", want: "read"},
		{name: "redirect &>", cmd: "&> /tmp/out.txt ls", want: "ls"},
		{name: "redirect &>>", cmd: "&>> /tmp/out.txt ls", want: "ls"},
		// A here-document's body follows on the next lines, up to a delimiter
		// line the search does not look for: the word after it may be body
		// text.
		{name: "heredoc names nothing", cmd: "<<EOF cat"},
		// A `(` where a redirect's target belongs is a process substitution
		// or a zsh glob ((a|b), (x).csv), and the tokenizer cannot tell
		// which: nothing on the line is certainly in command position.
		// Consuming the paren as the target once read the path as the
		// program.
		{name: "process substitution", cmd: "<(cat /home/alice/secret.csv) ls"},
		{name: "zsh glob as a redirect target", cmd: "> (/home/alice/secret.csv) ls"},
		// A separator where a target should be is a syntax error to both
		// shells, and nothing on the line runs. This once named ls, reading
		// on to the word after the separator -- which named secret.csv for
		// `> ; /home/alice/secret.csv x`, a line no rule tells from this one.
		{name: "redirect then separator", cmd: "> ; ls /home/alice/secret.csv"},
		// Nor is another redirect, which owns the word after it: `> >` is a
		// syntax error to both shells, and naming ls would name a command
		// that never runs. The same rule stops `> > f SECRETARG` naming its
		// argument.
		{name: "redirect then redirect", cmd: "> > /home/alice/secret.csv ls"},
		{name: "redirect then redirect, then an argument", cmd: "> > /home/alice/secret.csv SECRETARG"},
		{name: "redirect then redirect, then a path", cmd: "> > x /home/alice/secret-thing"},
		// A process substitution is a word, and so a target: `cat < <(ls)`
		// is an ordinary idiom. Only glued is `<(` one; spaced, the `<` is a
		// redirect with no target.
		{name: "a process substitution as a redirect's target", cmd: "cat > >(tee log)", want: "cat"},
		{name: "a process substitution as an input redirect's target", cmd: "cat < <(ls)", want: "cat"},
		{name: "a paren spaced from its < is not a process substitution", cmd: "cat > > (tee log)"},
		// A blank splits `<>` into `<` and whatever follows, judged as a
		// target like any other: here a process substitution, which both
		// shells read.
		{name: "a spaced < then a process substitution", cmd: "ls < >(x) y", want: "ls"},
		// A comment where the target belongs: the rest of the line is the
		// comment, and the redirect has no target.
		{name: "a comment where a redirect target belongs", cmd: "> #SECRET ls"},
		{name: "a comment where a redirect target belongs, after the command", cmd: "ls > #x"},
		// The tokenizer cannot tell `FOO=1 ( ls )` (which bash rejects) from
		// the array assignment `FOO=( ls )`, whose words are data, so an
		// assignment followed by `(` names nothing.
		{name: "assignment then paren names nothing", cmd: "FOO=1 ( ls )"},

		// Every construct below leaked data as the program when the rule was
		// "skip what cannot be a program and take the next word": the next
		// word was inside something the skip did not parse. Main recorded a
		// metacharacter for each -- useless, but no content. Null is the
		// answer wherever the command position cannot be found for certain.
		{name: "array assignment", cmd: "arr=(/home/alice/customer-list.csv /x) ; ls"},
		{name: "array assignment, no space", cmd: "files=(SECRETWORD other); echo"},
		{name: "heredoc body", cmd: "<<EOF\nSECRETBODY line\nEOF"},
		{name: "redirect then heredoc body", cmd: "> /tmp/out.txt <<'EOF'\nSECRETBODY hunter2\nEOF"},
		{name: "heredoc with dash", cmd: "<<-EOF\n\tSECRETBODY\nEOF"},
		{name: "assignment then heredoc", cmd: "X=1 <<EOF\nSECRETBODY\nEOF"},
		{name: "arithmetic substitution", cmd: "n=$(( 4111111111111111 % 97 ))"},
		{name: "arithmetic command", cmd: "(( SECRETVAR > 3 ))"},
		{name: "command substitution in assignment", cmd: "n=$(cat /home/alice/secret.csv)"},
		{name: "backtick in assignment", cmd: "x=`cat /SECRET`"},
		{name: "backtick in assignment, with args", cmd: "COUNT=`wc -l /home/alice/customer-list.csv`"},
		{name: "append assignment", cmd: "PATH+=:/home/alice/SECRETDIR"},
		{name: "append assignment, bare", cmd: "x+=SECRET"},
		{name: "subscript assignment", cmd: "arr[0]=SECRET ls", want: "ls"},
		// Bash would still assign here, but a quoted byte before the `=` is
		// what makes arr[0]"="x a command, and the two are refused alike.
		{name: "quoted subscript assignment names nothing", cmd: `arr["k"]=v ls`},
		{name: "quoted value is still an assignment", cmd: `FOO="a b" ls`, want: "ls"},
		{name: "a comment", cmd: "#SECRET comment"},
		{name: "zsh >&|", cmd: ">&| /home/alice/secret.csv ls", want: "ls"},
		{name: "zsh >>|", cmd: ">>| /home/alice/secret.csv ls", want: "ls"},
		{name: "zsh >>&", cmd: ">>& /home/alice/secret.csv ls", want: "ls"},
		{name: "zsh >>&|", cmd: ">>&| /home/alice/secret.csv ls", want: "ls"},
		// Quoted, `{` is not the brace keyword but the command itself, and
		// `FOO=bar` is not an assignment: the shell runs a command by that
		// name, so the word after either is an argument.
		{name: "quoted brace is the command", cmd: "'{' /home/alice/SECRET.csv", want: "{"},
		{name: "quoted assignment is the command", cmd: "'FOO=bar' SECRET", want: "FOO=bar"},
		// A whole command line quoted into one word is still the command's
		// name to the shell, and it is the whole line: a word with a space
		// in it names nothing here, where path.Base would name "passwd".
		{name: "quoted command line", cmd: `"cat /etc/passwd"`},
		{name: "quoted command line with an assignment", cmd: `"MSG=Q3 is not public git commit"`},
		{name: "quoted word with a newline", cmd: "\"X=1 <<EOF\nsecret body\nEOF\""},

		// One group per class of line below, each named after the rule
		// that closes it.
		//
		// Operator pieces with a blank or a newline between them are two
		// operators to both shells: `> |` is a redirect with no target and
		// then a pipe, a syntax error. The tokenizer threw the blank away and
		// read them as `>|`, took the next word for the target and the word
		// after for the program -- hunter2 and -rf below.
		{name: "blank between > and |", cmd: "> | grep hunter2 f"},
		{name: "blank between > and &", cmd: "> & rm -rf /srv/acme"},
		{name: "newline between > and |", cmd: ">\n|tee /home/a/secret.log"},
		{name: "blank between << and <", cmd: "<< <& secret-fd cmd",
			why: "`<< <` is not the here-string `<<<`: `<<` is a here-document"},
		// Glued, they are one operator, as they always were.
		{name: "glued >| unchanged", cmd: ">| out ls", want: "ls"},
		{name: "glued >& unchanged", cmd: ">& f ls", want: "ls"},
		{name: "glued 2>&1 after a command unchanged", cmd: "make 2>&1 | tee build.log", want: "make"},
		{name: "glued <<< unchanged", cmd: "<<< word cat", want: "cat"},

		// zsh reads <n-m> and <-> inside a word as part of it, and both
		// shells read a glued <( ) or >( ) the same way, so the tokenizer's
		// first piece is a fragment of the command word, not the word.
		{name: "numeric glob inside the command word", cmd: "/home/a/acme-<1-3>/run.sh"},
		{name: "numeric glob after a bare word", cmd: "x<1-9> SECRET"},
		{name: "open numeric glob in a path", cmd: "/home/a/<->/run"},
		{name: "process substitution glued to the command word", cmd: "/home/a/acme-merger<(true) --flag"},
		// A word ending in `/` that runs into an operator is a directory
		// fragment of whatever the shell reads there.
		{name: "a directory run into an operator", cmd: "/home/alice/acme-q3/<x>/run"},
		// Not glued, a process substitution is an argument like any other.
		{name: "spaced process substitution is an argument", cmd: "diff <(sort a) <(sort b)", want: "diff"},
		// Each glue runsOn asks for, alone: a numeric glob spaced from the
		// command word is an argument, and a `<` spaced from the digits
		// after it is an input redirect from the file 1-3.
		{name: "a numeric glob spaced from the command word", cmd: "cat <1-3> x", want: "cat"},
		{name: "digits spaced from a glued <", cmd: "cat< 1-3 x", want: "cat"},

		// A computed or globbed command word is whatever the expansion
		// yields: $x:gs/SECRET// runs $x with SECRET removed, and path.Base
		// read the substitution's delimiters as a path.
		{name: "zsh modifier on an unbraced parameter", cmd: "$x:gs/SECRET//"},
		{name: "a variable as the command", cmd: "$cmd --prod"},
		{name: "quoted zsh modifier", cmd: `"$x:s/SECRET/"`},
		{name: "glob with zsh exclusion", cmd: "*/deploy~*/SECRET"},
		{name: "path glob with zsh exclusion", cmd: "./bin/*~*/acme"},
		// Each glob character alone, so no one of the rules rides on another.
		{name: "exclusion without a star", cmd: "/opt/deploy~/opt/SECRET"},
		{name: "a star alone", cmd: "/home/alice/acme-merger-* --prod"},
		{name: "a question mark alone", cmd: "/home/alice/acme-q? --prod"},
		{name: "a leading tilde is a home directory", cmd: "~/bin/tool --flag", want: "tool"},
		{name: "a variable in an argument is not the command", cmd: "echo $HOME", want: "echo"},
		// An all-digit command word: an fd number the tokenizer split from
		// its redirect, or a word after a separator that was not one.
		{name: "all-digit word before &>", cmd: "2&>/dev/null ls"},
		{name: "all-digit word after a blank-split > &", cmd: "> & 2 SECRETARG"},

		// A byte the shell does not split on but the tokenizer did, or split
		// on where the shell does not: the word is not the word the shell
		// runs. A NUL truncates the line outright.
		{name: "a NUL", cmd: "\x00#hunter2 ls"},
		{name: "a vertical tab", cmd: "cat\v/home/a/s.csv"},
		{name: "a line joined by U+00A0", cmd: "git\u00a0push\u00a0origin\u00a0x"},
		{name: "a NUL where a redirect target belongs", cmd: ">\x00 /home/alice/secret.csv"},

		// Words then `()` define functions, whatever sits between the names.
		{name: "function definition with a redirect between the names", cmd: "deploy_acme >x deploy_prod () { ls; }"},
		{name: "function definition with an expansion between the names", cmd: "deploy_acme $(date) () { ls; }"},
		{name: "a separator ends the search for ()", cmd: "ls & f () { :; }", want: "ls"},
		{name: "a newline ends the search for ()", cmd: "cd /tmp\nf () { ls; }", want: "cd",
			why: "a newline ends the command, so the next line's () defines a function and does not stop cd running"},
		// A $'...' holding an escaped quote is one closed word to the shell;
		// read to the first quote of any kind, it hid the () after it.
		{name: "an escaped quote inside $'...'", cmd: "deploy_acme $'\\'' () { ls; }"},
		{name: "an unterminated quote still names what came before it", cmd: `echo "unterminated`, want: "echo"},

		// Every separator ends the search for (): each one here, alone, is
		// all that stands between the command word and the () after it.
		{name: "; ends the search for ()", cmd: `set -e; die() { echo "$1"; exit 1; }`, want: "set"},
		{name: "&& ends the search for ()", cmd: "make && f() { ls; }; f", want: "make"},
		{name: "|| ends the search for ()", cmd: "make || f () { :; }", want: "make"},
		{name: "| ends the search for ()", cmd: "ls | f () { :; }", want: "ls"},
		// Except inside a group -- $( ), <( ), >( ), a glob's @( ), or
		// backticks -- which is skipped to its close first: a separator or
		// a newline inside one ends a command the shell has not reached
		// the end of yet.
		{name: "a ; inside $( ) does not end the search", cmd: "deploy_acme $(x; y) () { ls; }"},
		{name: "a ; inside <( ) does not end the search", cmd: "deploy_acme <(x; y) () { ls; }"},
		{name: "a ; inside >( ) does not end the search", cmd: "deploy_acme >(x; y) () { ls; }"},
		{name: "a ; inside backticks does not end the search", cmd: "deploy_acme `x; y` () { ls; }"},
		{name: "a | inside a glob group does not end the search", cmd: "deploy_acme @(a|b) () { ls; }"},
		{name: "a ; inside a redirect target's $( ) does not end the search", cmd: "deploy_acme > $(echo x; true) deploy_prod () { ls; }"},
		{name: "a newline inside $( ) does not end the search", cmd: "deploy_acme $(x\ny) () { ls; }"},
		{name: "groups close by depth", cmd: "deploy_acme $(a $(b); c) () { ls; }"},
		{name: "backticks inside $( )", cmd: "deploy_acme $(a `b; c`; d) () { ls; }"},
		{name: "a group that never closes names nothing", cmd: "echo $(date; ls"},
		// A closed group is skipped and no more: what comes after it is
		// read as before.
		{name: "a separator after a closed group ends the search", cmd: "deploy_acme $(x; y); f () { ls; }", want: "deploy_acme"},
		{name: "a separator after closed backticks ends the search", cmd: "echo `date; ls`; f () { :; }", want: "echo"},
		{name: "a quoted backtick opens no group", cmd: "echo '`'; f () { :; }", want: "echo"},
		{name: "an escaped backtick opens no group", cmd: "echo \\`; f () { :; }", want: "echo"},
		// &> is a redirect only glued: `& >x` is the background separator
		// and then a redirect, and the command before it is complete.
		{name: "&> between function names is a redirect", cmd: "f &>x g () { :; }"},
		{name: "& then > is a separator and a redirect", cmd: "f & >x g () { :; }", want: "f"},
		// Inside backticks only a backtick counts: the ) here closes
		// nothing, and the ; does not end the command.
		{name: "a paren inside backticks closes nothing", cmd: "deploy_acme `a ); b` () { ls; }"},
		{name: "a paren inside backticks inside $( ) closes nothing", cmd: "deploy_acme $(a `b ); c` ) () { ls; }"},

		// Where the scan cannot tell a group's depth from the tokens -- a
		// case pattern's ), a comment, a here-document's body -- or cannot
		// see where a ${ ends, it cannot know where the command ends either.
		{name: "a case statement inside $( )", cmd: "deploy_acme $(case x in a) :;; esac) () { ls; }"},
		{name: "a comment inside $( )", cmd: "deploy_acme $(# )\n) () { ls; }"},
		{name: "a here-document inside $( )", cmd: "deploy_acme $(cat <<EOF\n)\nEOF\n) () { ls; }"},
		{name: "a newline inside $( ) after a here-document", cmd: "deploy_acme <<EOF $(x\n)\nEOF\n) () { ls; }"},
		{name: "bash 5.3's ${ cmd; }", cmd: "deploy_acme ${ x; y; } () { ls; }"},
		{name: "a paren inside ${ }", cmd: "deploy_acme ${x#)} () { ls; }"},
		// Each of those is refused for what it is, and no more.
		{name: "a closed ${ } is a word", cmd: "echo ${HOME}", want: "echo"},
		{name: "a quoted ${ is text", cmd: "echo '${' x", want: "echo"},
		{name: "a here-string inside $( ) is a word", cmd: "echo $(cat <<< x)", want: "echo"},
		{name: "a here-document outside a group", cmd: "cat <<EOF\nbody\nEOF", want: "cat"},

		// Where the lexer's own reading stops being certain, a command
		// still open there has an end nobody saw. Bash reads $$' as $$ and
		// a plain quote, zsh as $ and a $'...' string: below, zsh runs f
		// with one argument, and to bash the () makes the line an error.
		{name: "$$' read two ways", cmd: "f $$'\\' () { ls; }'"},
		{name: "$$' read two ways, unterminated one way", cmd: "f $$'\\' () { ls; } 'x'"},
		{name: "$$' read two ways, unterminated both ways", cmd: "deploy_acme $$'\\' () { ls; }"},
		{name: "a command ended before $$' keeps its name", cmd: "ls; echo $$'x'", want: "ls"},
		// A quote after an expansion the lexer does not parse may be no
		// quote to the shell: "$(echo '"')" is one word, and the () after
		// it is a function definition the tokens never reach.
		{name: "an unterminated quote inside an expansion's word", cmd: "deploy_acme \"$(echo '\"')\" () { ls; }"},
		{name: "an unterminated quote after an expansion's word", cmd: "deploy_acme \"$(echo \")'\")\" () { ls; }"},
		{name: "a trailing backslash after an expansion", cmd: "deploy_acme \"$(echo '\"')\" () { ls; } \"$(echo '\"')\" \\"},

		// A $( ) inside double quotes is one context of its own, in which
		// quotes nest: "$(echo "a;b")" is one word, and its ; ends nothing.
		// Read to the first inner quote, the outer string ended there and
		// the ; looked like a separator, so the () after it went unseen.
		{name: "a ; inside a quoted $( )'s own quotes", cmd: `hunter2 "$(echo "a;b")" () { ls; }`},
		{name: "a ; inside a quoted $( ) in a redirect target", cmd: `hunter2 >"$(echo ";")" x () { ls; }`},
		{name: "a ; inside backticks' own quotes", cmd: "hunter2 `echo \"a;b\"` () { ls; }"},
		{name: "a bare function definition", cmd: "hunter2 () { ls; }"},
		{name: "a newline inside a quoted $( )'s own quotes", cmd: "hunter2 \"$(echo \"a\nb\")\" () { ls; }"},
		{name: "a quoted $( ) nested in a quoted $( )", cmd: `hunter2 "$(echo "$(echo "a;b")")" () { ls; }`},
		// Where the end of the $( ) cannot be told -- a case pattern's ), a
		// comment, a here-document whose delimiter line is not one, a ${
		// hiding a paren -- nothing after it is certain.
		{name: "a case statement inside a quoted $( )", cmd: `hunter2 "$(case x in a) echo "a;b";; esac)" () { ls; }`},
		{name: "a comment inside a quoted $( )", cmd: "hunter2 \"$(echo \"a;b\" # )\n)\" () { ls; }"},
		{name: "a here-document delimiter glued to the close", cmd: "hunter2 \"$(cat <<EOF\na;b\nEOF)\" () { ls; }"},
		{name: "a paren inside ${ } inside a quoted $( )", cmd: `hunter2 "$(echo ${x#)} "a;b")" () { ls; }`},
		{name: "a quoted $( ) that never closes", cmd: `hunter2 "$(echo "a;b" () { ls; }`},
		// A backslash-newline is literal in a quoted here-document's body,
		// and joining it made EO\<newline>F the delimiter line: the body's
		// "a;b" was then read as code, and its ; as a separator.
		{name: "a continuation inside a quoted $( )'s here-document", cmd: "hunter2 \"$(cat <<'EOF'\nEO\\\nF\n)\n\"a;b\"\nEOF\n)\" () { ls; }"},
		{name: "a continuation after a quoted $( )", cmd: "cd \"$(git rev-parse --show-toplevel)\" && \\\nmake", want: "make"},
		// And every line that ran a command before still names it.
		{name: "a quoted $( ) with its own quotes", cmd: `echo "$(date "+%Y")" done`, want: "echo"},
		{name: "a separator after a quoted $( )", cmd: `echo "$(date "+%Y")"; f () { :; }`, want: "echo"},
		{name: "the commit-message here-document", cmd: "git commit -m \"$(cat <<'EOF'\nfix(shape): it's \"done\" (mostly\n\n# a ) and a ; case in point\nEOF\n)\"", want: "git"},
		{name: "a quoted $( ) then &&", cmd: `cd "$(git rev-parse --show-toplevel)" && make`, want: "make"},
		{name: "a closed ${ } inside a quoted $( )", cmd: `cd "$(dirname "${BASH_SOURCE[0]}")" && pwd`, want: "pwd"},
		// $[ ] is arithmetic, a pair of its own: a paren inside it is no
		// group, and counting one moves the close.
		{name: "a ( inside $[ ] inside a quoted $( )", cmd: `hunter2 "$(: $[ ( ] )" () { echo ")"; }; x ""`},
		{name: "a ) inside $[ ] inside a quoted $( )", cmd: `hunter2 "$(: $[ ) ] "a;b")" () { ls; }`},
		// A ' inside a nested "..." is no quote to the shell; read as one, it
		// stopped the joining of every backslash-newline after it, and the
		// case, << or ${ they split went unseen.
		{name: "a split case after a nested \"'\"", cmd: "hunter2 \"$(: \"'\"; ca\\\nse x in a) : \"x;y\";; esac)\" () { ls; }"},
		{name: "a split ${ after a nested \"'\"", cmd: "hunter2 \"$(: \"'\" $\\\n{x#)} \"a;b\")\" () { ls; }"},
		{name: "a split << after a nested \"'\"", cmd: "hunter2 \"$(echo \"'\"; cat <\\\n<EOF\n)\"a;b\"\nEOF\n)\" () { ls; }"},
		{name: "a continuation inside a single-quoted string in a quoted $( )", cmd: "hunter2 \"$(echo \"x\" 'a\\\nb' \"a;b\")\" () { ls; }"},
		// Inside double quotes an even run of $ is $$ and then a byte: "$$("
		// opens nothing in bash, and a substitution only by misreading.
		{name: "$$( inside double quotes", cmd: `hunter2 "$$(x"'")" ; "'"" () { ls; }`},
		{name: "$$( inside a quoted $( )", cmd: `hunter2 "$(: "$$(x")" "a;b")" () { ls; }`},
		// Arithmetic, where << is a shift and a ( no group, is read as such
		// only where it cannot be a subshell instead.
		{name: "a shift inside a quoted $(( ))", cmd: `echo "$((1<<3))"`, want: "echo"},
		{name: "a shift inside $(( )) inside a quoted $( )", cmd: `echo "$(echo $(( 1 << n )))" done`, want: "echo"},
		{name: "a ${ } inside a quoted $(( ))", cmd: `echo "$(( ${#arr[@]} - 1 ))"`, want: "echo"},
		{name: "a quoted $( ) inside a quoted $(( ))", cmd: `echo "$(( ( $(date -d '10:00' '+%s') - $(date '+%s') ) / 60 ))"`, want: "echo"},
		{name: "a subshell inside a quoted $( (", cmd: `hunter2 "$((echo "a;b") )" () { ls; }`},
		{name: "a ; inside a quoted $(( ))", cmd: `hunter2 "$((1;"a;b"))" () { ls; }`},
		// The word case is a keyword only where a command begins.
		{name: "case as an argument inside a quoted $( )", cmd: `echo "$(grep -i case file)"`, want: "echo"},
		{name: "case after an arithmetic expansion", cmd: `echo "$(echo $((1))case)"`, want: "echo"},
		{name: "case after a separator inside a quoted $( )", cmd: `hunter2 "$(grep x; case x in a) "a;b";; esac)" () { ls; }`},
		{name: "case after a keyword inside a quoted $( )", cmd: `hunter2 "$(if :; then case x in a) "a;b";; esac; fi)" () { ls; }`},
		{name: "case after an assignment inside a quoted $( )", cmd: `hunter2 "$(x=1 case x in a) "a;b";; esac)" () { ls; }`},
		{name: "case after time -p inside a quoted $( )", cmd: `hunter2 "$(time -p case x in a) "a;b";; esac)" () { ls; }`},
		{name: "case after a redirect inside a quoted $( )", cmd: `hunter2 "$(>f case x in a) "a;b";; esac)" () { ls; }`},
		// A backslash-newline outside a here-document says nothing about its
		// body.
		{name: "a continuation after a quoted here-document", cmd: "git commit -m \"$(cat <<'EOF'\nfix\nEOF\n)\" && \\\ngit push", want: "git"},
		{name: "a continuation before a quoted here-document", cmd: "git add -A && \\\ngit commit -m \"$(cat <<'EOF'\nfix\nEOF\n)\"", want: "git"},
		{name: "a continuation on the here-document's command line", cmd: "git commit -m \"$(cat <<'EOF' | \\\ntr a b\nfix\nEOF\n)\"", want: "git"},
		{name: "a continuation inside an unquoted here-document", cmd: "hunter2 \"$(cat <<EOF\nEO\\\nF\n)\n\"a;b\"\nEOF\n)\" () { ls; }"},

		// A redirect split from a piece of its operator, or with no word
		// where its target belongs, is a syntax error to both shells, and a
		// line holding one runs nothing -- wherever the search meets it.
		{name: "a split operator between function names", cmd: "deploy_acme > | x deploy_prod () { ls; }"},
		{name: "a split operator after the command word", cmd: "ls < > /home/alice/secret.csv"},
		{name: "a redirect with ; for its target", cmd: "> ; /home/alice/secret.csv x"},
		{name: "a redirect with | for its target", cmd: "< | /home/alice/secret.csv x"},
		{name: "a redirect with && for its target", cmd: ">> && /home/alice/secret.csv x"},
		{name: "a redirect with no target after the command word", cmd: "ls > ; x"},
		{name: "a redirect at the end of the line", cmd: "ls >"},
		{name: "a redirect at the end of a line", cmd: "ls >\nx"},

		// A word that can never name a program is not a command word.
		{name: "a directory", cmd: "/home/alice/acme-q3/ --prod"},
		{name: "a user's home directory", cmd: "~customer-acme --prod"},
		{name: "zsh's ~+", cmd: "~+ x"},
		{name: "zsh's ~-", cmd: "~- x"},
		{name: "zsh's ^ negation", cmd: "/usr/bin/^SECRET"},
		{name: "zsh's # repetition", cmd: "/usr/bin/SECRET# x"},
		{name: "an empty quoted word", cmd: "'' SECRET"},
		{name: "an empty double-quoted word", cmd: `"" SECRET`},
		{name: "a user's home directory then a path", cmd: "~alice/bin/tool x", want: "tool"},
		{name: "a path ending in .", cmd: "/home/alice/acme-q3/."},
		{name: "a path ending in ..", cmd: "/home/alice/acme-q3/.."},
		{name: "the source builtin", cmd: ". ./venv/bin/activate", want: "."},
		{name: "a jobspec", cmd: "%hunter2"},
		// A quoted word whose only blank is a tab or a newline is a command
		// line all the same, and neither byte is a control byte to the
		// line: the command word's own rule refuses it.
		{name: "a quoted command line split by a tab", cmd: "\"cat\t/home/alice/secret.csv\""},
		{name: "a quoted command line split by a newline", cmd: "\"cat\n/home/alice/secret.csv\""},
		// Refusing every byte outside printable ASCII in the command word
		// refuses a real program in a directory with such a name: a path
		// through /Users/Müller names nothing. U+00A0 and its kind cannot
		// be told from a letter byte by byte, and naming a word that holds
		// one recorded a whole command line.
		{name: "a program under a non-ASCII directory", cmd: "/Users/M\u00fcller/.local/bin/claude"},

		// A control byte anywhere in the line, not only in the command word.
		{name: "a control byte where a redirect target belongs", cmd: ">\x01 /home/alice/secret.csv"},
		{name: "an escape byte between words", cmd: "ls \x1b x"},
		{name: "a DEL inside the command word", cmd: "cat\x7f/home/alice/secret.csv"},
		{name: "a DEL where a redirect target belongs", cmd: ">\x7f /home/alice/secret.csv"},
		// Tab and newline are the shell's own separators, not control bytes.
		{name: "a tab between words", cmd: "ls\t-la", want: "ls"},

		// Ordinary lines keep their program.
		{name: "ordinary ls", cmd: "ls -la", want: "ls"},
		{name: "ordinary git", cmd: "git status", want: "git"},
		{name: "ordinary cd", cmd: "cd /tmp && make", want: "make"},
		{name: "a redirect glued to the command", cmd: "ls>out", want: "ls"},
		{name: "&> after the command", cmd: "make &>/dev/null", want: "make"},
		{name: "&> then a separator", cmd: "npm test &> build.log && echo ok", want: "npm"},
		{name: "a pipe glued to a subshell", cmd: "ls|(cat)", want: "ls"},
		{name: "a redirect glued to digits and a dash", cmd: "make>1-3", want: "make"},
		{name: "an absolute path in command position", cmd: "/usr/local/bin/go build", want: "go"},

		// Unchanged behaviour, asserted so the fix cannot quietly move it.
		{name: "ordinary command", cmd: "cd /tmp && go build", want: "go"},
		{name: "assignments skipped", cmd: "FOO=bar BAZ=qux make -j4", want: "make"},
		{name: "assignment after the program is an argument", cmd: "env FOO=bar", want: "env"},
		{name: "path is reduced to its base", cmd: "/usr/local/bin/go build", want: "go"},

		// Null is the honest answer, and must stay null rather than become
		// a metacharacter.
		{name: "nothing but assignments", cmd: "FOO=bar"},
		{name: "nothing but operators", cmd: "&& ; |"},
		{name: "empty", cmd: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]string{"command": tc.cmd})
			if err != nil {
				t.Fatal(err)
			}
			got := Derive("Bash", raw, []byte("key"))

			if tc.want == "" {
				if got.Program != nil {
					t.Errorf("program for %q is %q, want null", tc.cmd, *got.Program)
				}
				return
			}
			if got.Program == nil {
				t.Fatalf("program for %q is null, want %q", tc.cmd, tc.want)
			}
			if *got.Program != tc.want {
				msg := ""
				if tc.why != "" {
					msg = "\n  " + tc.why
				}
				t.Errorf("program for %q is %q, want %q%s", tc.cmd, *got.Program, tc.want, msg)
			}
		})
	}
}

// TestProgramIONumberNamesNothing: an fd number or fd variable written against
// its redirect -- `2> out.txt ls`, `{fd}>out ls` -- names no program.
//
// `2> out.txt ls` redirects fd 2 and runs ls; `2 > out.txt ls` runs a command
// named 2 and hands it ls as an argument. The tokens tell the two apart by
// whether the `>` is glued to the 2, but the search does not read on past an
// fd prefix to the command after it: it names nothing, and "2" -- a count, not
// a program -- is never the answer either.
func TestProgramIONumberNamesNothing(t *testing.T) {
	for _, cmd := range []string{"2> out.txt ls", "{fd}>out ls", ">/dev/null 2>&1 command -v git"} {
		raw, err := json.Marshal(map[string]string{"command": cmd})
		if err != nil {
			t.Fatal(err)
		}
		if got := Derive("Bash", raw, []byte("key")); got.Program != nil {
			t.Errorf("program for %q is %q, want null", cmd, *got.Program)
		}
	}
}

// TestArgcExcludesLeadingAssignments pins what argc counts, which changed
// without notice once: finding the program by skipping a `FOO=bar` prefix
// stopped dropping that prefix from the tokens argc is taken over, and
// `FOO=bar BAZ=qux make -j4` went from 2 to 4 while its digest stayed the same.
// The same command then counts differently depending on which build recorded
// it, in a field the store schema publishes.
//
// argc is over the whole line after a leading assignment prefix: operators and
// every stage of a pipeline count, and an assignment after the program is an
// argument like any other.
func TestArgcExcludesLeadingAssignments(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want int
	}{
		{"FOO=bar BAZ=qux make -j4", 2},
		{"FOO=1 ls", 1},
		{"FOO=bar", 0},
		{"env FOO=bar", 2},
		{"ls | wc -l", 4},
		{"( cd /tmp && ls )", 6},
	} {
		raw, err := json.Marshal(map[string]string{"command": tc.cmd})
		if err != nil {
			t.Fatal(err)
		}
		got := Derive("Bash", raw, []byte("key"))
		if got.Argc == nil {
			t.Errorf("argc for %q is null, want %d", tc.cmd, tc.want)
			continue
		}
		if *got.Argc != tc.want {
			t.Errorf("argc for %q is %d, want %d", tc.cmd, *got.Argc, tc.want)
		}
	}
}

// TestProgramLooksPastADirectoryChange: `cd` is where a command runs, not what
// it runs. On a real session 1,397 of 1,495 shell calls opened with
// `cd … &&`, and "by program" read "cd 1397" -- the field said nothing. Only
// `&&` and `;` are followed; anything else after `cd` is left alone rather
// than guessed at, and a command after `cd` that cannot be told is null.
func TestProgramLooksPastADirectoryChange(t *testing.T) {
	for _, tc := range []struct {
		cmd, want string
	}{
		{"cd /x && go test ./...", "go"},
		{"cd /x; make", "make"},
		{"cd a && cd b && npm test", "npm"},
		{"( cd x; make )", "make"},
		{"cd /x && FOO=1 git log", "git"},
		{"cd /x", "cd"},
		{"cd /x | wc -l", "cd"},
		{"cd /x || exit 1", "cd"},
		// The command after the separator is held to the same rules as a
		// line's first command.
		{"cd /x && $EDITOR f", ""},
		{"cd /x && hunter2 () { ls; }", ""},
		{`cd /x && hunter2 "$(echo "a;b")" () { ls; }`, ""},
		// The separator is the one that ends cd's command: not one inside a
		// group, and not one in a comment, which leaves cd the whole command.
		{"cd $(echo; hunter2) && make", "make"},
		{"cd `echo; hunter2` && make", "make"},
		{"cd /x # && hunter2", "cd"},
		{"cd /x 2>/dev/null && make", "make"},
	} {
		in, _ := json.Marshal(map[string]string{"command": tc.cmd})
		got := Derive("Bash", in, []byte("k")).Program
		switch {
		case tc.want == "" && got != nil:
			t.Errorf("%q: program = %q, want null", tc.cmd, *got)
		case tc.want != "" && (got == nil || *got != tc.want):
			t.Errorf("%q: program = %v, want %q", tc.cmd, got, tc.want)
		}
	}
}

// TestArgcKeepsItsReadingOfANSICQuotes: the program search reads $'...' to its
// first unescaped quote, as the shell does, and argc does not. Counted that
// way, a $'...' holding an escaped quote is an open quote and argc is null, as
// it always was; counted the new way these would be 8 and 3, and the same
// command would count differently depending on which build recorded it.
func TestArgcKeepsItsReadingOfANSICQuotes(t *testing.T) {
	for _, cmd := range []string{`f $'\'' () { ls; }`, `echo $'it\'s' done`} {
		raw, err := json.Marshal(map[string]string{"command": cmd})
		if err != nil {
			t.Fatal(err)
		}
		if got := Derive("Bash", raw, []byte("key")); got.Argc != nil {
			t.Errorf("argc for %q is %d, want null", cmd, *got.Argc)
		}
	}
	// Nor does it stop where the program search does: bash and zsh read $$'
	// differently and the search names nothing past it, but the count is
	// the one it has always been.
	for _, tc := range []struct {
		cmd  string
		want int
	}{
		{`echo $$'x'`, 2},
		{`ls $$'\' x`, 3},
	} {
		raw, err := json.Marshal(map[string]string{"command": tc.cmd})
		if err != nil {
			t.Fatal(err)
		}
		got := Derive("Bash", raw, []byte("key"))
		if got.Argc == nil || *got.Argc != tc.want {
			t.Errorf("argc for %q is %v, want %d", tc.cmd, got.Argc, tc.want)
		}
	}
}
