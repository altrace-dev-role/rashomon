package main

// beforeSetupWrite runs just before graph index creates the configuration it
// inferred, with the root: a variable so a test can put a racing writer's
// file on the name between the read that found none and the create.
var beforeSetupWrite = func(string) {}
