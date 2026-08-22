package pl.m2x.commandsspy;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * Beta 1.7.3 has two command seams and they disagree about the leading slash:
 * ServerPlayNetworkHandler#handleCommand receives "/me waves", while
 * ServerCommandHandler#executeCommand receives "save-all". Every other loader hands
 * CommandsSpy.handleCommand a bare name, so normalizing happens here and never in the
 * shared core. See docs/superpowers/specs/2026-08-22-babric-loader-support-spec.md.
 */
class CommandsSpyBabricNormalizeTest {

    @Test
    void stripsALeadingSlashFromThePlayerSeam() {
        assertEquals("me waves", CommandsSpyBabric.normalize("/me waves"));
    }

    @Test
    void leavesTheConsoleSeamUnchanged() {
        assertEquals("save-all", CommandsSpyBabric.normalize("save-all"));
    }

    @Test
    void stripsOnlyTheFirstSlash() {
        assertEquals("/me waves", CommandsSpyBabric.normalize("//me waves"));
    }

    @Test
    void leavesABareSlashAsTheEmptyString() {
        assertEquals("", CommandsSpyBabric.normalize("/"));
    }

    @Test
    void leavesTheEmptyStringAlone() {
        assertEquals("", CommandsSpyBabric.normalize(""));
    }

    @Test
    void leavesASlashThatIsNotLeadingAlone() {
        assertEquals("tp Steve /home", CommandsSpyBabric.normalize("tp Steve /home"));
    }
}
