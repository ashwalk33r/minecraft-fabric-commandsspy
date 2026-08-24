package pl.m2x.commandsspy;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * BTA strips the leading slash before dispatching, so the seam normally delivers a bare
 * line - but normalize() is applied anyway, so the mod's output cannot change if a later
 * BTA release stops stripping. Contract is identical to CommandsSpyBabric.normalize.
 */
class CommandsSpyBtaNormalizeTest {

    @Test
    void leavesTheStrippedSeamLineUnchanged() {
        assertEquals("nickname set Bot", CommandsSpyBta.normalize("nickname set Bot"));
    }

    @Test
    void stripsALeadingSlashIfBtaEverStopsStripping() {
        assertEquals("me waves", CommandsSpyBta.normalize("/me waves"));
    }

    @Test
    void stripsOnlyTheFirstSlash() {
        assertEquals("/me waves", CommandsSpyBta.normalize("//me waves"));
    }

    @Test
    void leavesABareSlashAsTheEmptyString() {
        assertEquals("", CommandsSpyBta.normalize("/"));
    }

    @Test
    void leavesTheEmptyStringAlone() {
        assertEquals("", CommandsSpyBta.normalize(""));
    }

    @Test
    void leavesASlashThatIsNotLeadingAlone() {
        assertEquals("tp Steve /home", CommandsSpyBta.normalize("tp Steve /home"));
    }
}
