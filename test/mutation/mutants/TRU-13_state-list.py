# TRU-13: a steer line's state list carries `unconfirmed`, the state of a line
# whose delivery the record could not confirm. Dropping it from the list the
# log writer and reader share is the whole regression.
m("TRU-13 the steer line state list loses unconfirmed", "internal/steer/steer.go",
  "\t\tLineStateExpired, LineStateUnconfirmed,\n",
  "\t\tLineStateExpired,\n", "TestTRU13b_")
