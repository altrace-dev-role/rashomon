# WM-4.6r: the open-requirement lines (internal/graph/session/openreqs.go). OpenReqsLines lists the
# opened_unit requirements, open or superseded, whose path the path slot's rule takes; returns nil when it
# lists none, else two lines: at most two quoted names, open rows first and superseded rows marked after
# them, newest first and ties by path, then "and N more" counted over both groups, and the sentence for one,
# two or more rows. A path the rule refuses is neither named nor counted. Each break below is one of those
# promises undone. The prefix is WM-4.6r, not WM-4.6, so the file never shares a prefix with the plan item
# that numbers itself W-M4.6 (PREFIX-UNIQUE1).
m('WM-4.6r a path the slot rule refuses is listed', 'internal/graph/session/openreqs.go',
  '\treturn slot.Path(arg).Validate() == nil\n',
  '\treturn true\n',
  'TestOpenReqs_(KeyedDigest|RefusedPath)IsNotListed')
m('WM-4.6r a requirement of any predicate is listed', 'internal/graph/session/openreqs.go',
  'r.Pred != steer.PredOpenedUnit || !listable(r.Arg)',
  'steer.PredOpenedUnit == "" || !listable(r.Arg)',
  'TestOpenReqs_OtherPredicatesAreNeverRendered')
m('WM-4.6r a met_once requirement is listed', 'internal/graph/session/openreqs.go',
  '\t\tcase ReqOpen:\n',
  '\t\tcase ReqOpen, ReqMetOnce:\n',
  'TestOpenReqs_MetOnceIsNeverRendered')
m('WM-4.6r a superseded requirement is not listed', 'internal/graph/session/openreqs.go',
  '\t\tcase ReqSuperseded:\n\t\t\tsuperseded = append(superseded, r)\n',
  '',
  'TestOpenReqs_SupersededAfterOpen')
m('WM-4.6r superseded rows come before open ones', 'internal/graph/session/openreqs.go',
  'slices.Concat(open, superseded)',
  'slices.Concat(superseded, open)',
  'TestOpenReqs_SupersededAfterOpen')
m('WM-4.6r a superseded name carries no mark', 'internal/graph/session/openreqs.go',
  '\t\tif r.Status == ReqSuperseded {\n\t\t\tname += supersededMark\n\t\t}\n',
  '',
  'TestOpenReqs_SupersededAfterOpen')
m('WM-4.6r the oldest row comes first', 'internal/graph/session/openreqs.go',
  'cmp.Compare(b.LastStatedAt, a.LastStatedAt)',
  'cmp.Compare(a.LastStatedAt, b.LastStatedAt)',
  'TestOpenReqs_NewestFirst')
m('WM-4.6r ties come in reverse path order', 'internal/graph/session/openreqs.go',
  'strings.Compare(a.Arg, b.Arg)',
  'strings.Compare(b.Arg, a.Arg)',
  'TestOpenReqs_TiesByPath')
m('WM-4.6r N counts the open group alone', 'internal/graph/session/openreqs.go',
  'more := n - len(names)',
  'more := len(open) - len(names)',
  'TestOpenReqs_MoreCountsBothGroups')
m('WM-4.6r three names print', 'internal/graph/session/openreqs.go',
  'const maxOpenNames = 2',
  'const maxOpenNames = 3',
  'TestOpenReqs_RowCountForms')
m('WM-4.6r two rows take the sentence of three', 'internal/graph/session/openreqs.go',
  '\tcase 2:\n\t\tsecond = "units rashomon named this session; no write under either seen since."\n',
  '',
  'TestOpenReqs_RowCountForms')
m('WM-4.6r no row returns an empty list, not nil', 'internal/graph/session/openreqs.go',
  '\tif n == 0 {\n\t\treturn nil\n\t}\n',
  '\tif n == 0 {\n\t\treturn []string{}\n\t}\n',
  'TestOpenReqs_NoRowNoLine')
m('WM-4.6r a name prints bare, outside the quote', 'internal/graph/session/openreqs.go',
  'name := slot.Path(r.Arg).Text()',
  'name := r.Arg',
  'TestOpenReqs_RowCountForms')
