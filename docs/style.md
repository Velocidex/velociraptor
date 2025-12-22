# Velociraptor code base style guide

The following summarises at a high level some style convensions in the
codebase.

## Receivers shall be named `self`

This convension is contrary to the usual Golang convention of using
different variable names for the receiver. In Velociraptor, all
receivers are named `self` in a similar way to the Python convention.

Using the same consistent name for all receivers makes code much
easier to read and reduces congnitive load. When you see `self` you
know it refers to the receiver of the current method.

## Variable naming

1. Local variables are named in snake case format. e.g: `fs_type`
2. Public Methods, functions and Types are named using Upper Camel
   Case. e.g. `MatchComponentPattern()`, `FSPathSpec`
3. Private Methods , Functions and types are names using lower camelCase

4. Struct methods should have the receiver defined as a pointer.

   This avoids copying the receiver on entering the function. The
   default golang behavior is responsible to many bugs (for example
   copying mutexes leading to race conditions). Additionally copying
   receivers may actually hide performance issues.

   It is just safer to pass receivers as pointers.

## Locking semantics

1. Minimize the use of recursive locks, preferring to use `sync.Mutex`
   instead.

2. Mutexes shall be embedded in the stucts they protect as a private
   variable. The variable shall be named `mu`. Hence locks are
   obtained using `self.mu.Lock()`

3. Public methods on structs that require locking shall obtain the
   lock themselves. If the struct needs to call internal private
   methods, by convention these should not acquire the lock.

   This implies that generally a method that starts with a lower case
   can only be called with the locks held (since they are private
   methods).

   Methods starting with a capital letter are safe to call without
   holding the lock.

For example consider the following struct:

```go
type ExampleType struct {
  mu           sync.Mutex
  ...
}

func (self *ExampleType) PublicMethod() {
  self.mu.Lock()
  defer self.mu.Unlock()

  self.privateMethod()
}

func (self *ExampleType) privateMethod() {
  .. do things with the lock acquired ...
}

func (self *ExampleType) otherMethod() {
  // Safe to call the private method which assume lock is held.
  self.privateMethod()
}
```

This scheme makes it easier to see which methods require the locks
held and eaiser to quickly audit the methods called within a function
to ensure they are safe:

* If a method starts with a lower case (private method) we know the
  lock must be held on entry to the method.
* All methods called from this method must also start with lower case.

Consistency in naming here ensures we do not deadlock due to recursive
locks.

## General guidelines

* Never use `panic()` in production code
