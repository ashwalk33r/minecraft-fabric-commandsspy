# Unit tests

`./gradlew test -PmcTarget=<target>` runs the suite against one era;
`make test` runs the offline routing contract plus all four targets. All four
run because each resolves its own Fabric loader and compile level — a pass on
one era proves nothing about another.

## How the suite boots

`fabric-loader-junit` boots a real FabricLoader for the test JVM (via a JUnit
Platform launcher-session listener), so `CommandsSpy`'s class initializer —
`CommandsSpyConfig.load()` → `FabricLoader.getInstance().getConfigDir()` —
runs untouched. No injectability seam exists in the mod; the fields are
public and the blacklist is mutable, which is all the tests need.

Log capture: `CommandsSpy.LOGGER` is declared as the log4j *core* `Logger`
(the implementation class), so `CapturingAppender` attaches and detaches
directly per test — no LoggerContext reconfiguration.

Side effects of a run: `config/commands-spy.json` and `logs/latest.log` in
the project directory (FabricLoader's config dir is real under
fabric-loader-junit).

## Isolation strategy

`CommandsSpy.CONFIG` and `CommandsSpy.BLACKLIST` are static finals wired at
class-load: `BLACKLIST` captures the very list instance `CONFIG.blacklist`
points at. Therefore:

- The blacklist must be mutated **in place** (`add`/`clear`). Reassigning
  `CONFIG.blacklist` swaps in a list `BLACKLIST` never sees, silently
  disabling the assertion.
- `CommandsSpyTestSupport.resetState()` runs from every `@BeforeEach`, making
  tests order-independent and immune to a config file left on disk.
- `CommandsSpyBlacklistTest.reflectsInPlaceMutationOfTheBackingList` guards
  the aliasing itself: if it fails, `resetState()` is a no-op and every
  blacklist assertion in the suite is vacuous.

## Known quirks (characterized, not endorsed)

`CommandsSpyCommand.getCommand` splits on the first U+0020 only, and only
when its index is `> 0`. The tests pin the fallout as-is:

1. A **leading space** means nothing is split off; the whole string becomes
   the "command name" and bypasses blacklist matching entirely.
2. A **tab-separated** command is never split.
3. Blacklist matching is **case-sensitive** and **exact**: `Say` does not
   suppress `say`; `say` does not cover `sayonara`; a multi-word entry like
   `say hello` can never match.
4. Empty input yields an empty command name.

Change the splitter and these tests are supposed to fail — update them
deliberately.

## Era-specific wiring

- **`src/test/resources/log4j2.xml` exists for classpath position, not
  logging policy.** fabric-loader-junit classifies whichever classpath entry
  first contains `log4j2.xml` as a system library; the 1.16.5 game jar
  bundles one at its root, so without this file the whole game jar is
  skipped and the mc114 test JVM dies with "couldn't locate the game". Do
  not delete it.
- **mc114 gets a modern `log4j-core` at test runtime only**: 1.16.5's log4j
  2.8.1 predates the ServiceLoader provider file the loader's classifier
  uses, so without the swap a second log4j-core loads on the knot
  classloader and the `core.Logger` cast dies with a
  same-name-different-classloader `ClassCastException`. The shipped jar is
  untouched; e2e proves the mod against the real 2.8.1.
- **`CapturingAppender` uses the deprecated 4-arg constructor** because
  mc114's log4j 2.8.1 lacks the 5-arg one; a single test tree compiles
  against every era.

## Static analysis on test sources

PMD runs the same categories on tests, with two rules excluded in
`config/pmd/test-ruleset.xml`: `UnitTestContainsTooManyAsserts` (caps a test
at one assertion) and `UnitTestAssertionsShouldIncludeMessage`. Everything
else still applies to test code. Coverage (jacoco) is deliberately off.
