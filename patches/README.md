# TDesign calendar draft preservation

`tdesign-vue-next@1.18.5.patch` fixes the date-range picker's input draft lifecycle.
Opening the popup is deferred. If typing starts first, the upstream visibility
watcher resets the draft cache to the applied value. Leaving a hovered date cell
then replaces the input with that stale cache. Resizing the calendar can trigger
the same mouse-leave event.

The patch initializes the popup from the current input and preserves raw drafts
in the hover cache, including incomplete or invalid text. Valid drafts are parsed
into dates before initializing the calendar and time columns. Invalid text never
drives those columns or becomes an applied range. Existing confirm, cancel, and
external-value synchronization remain responsible for the applied value.

The patch also resets a filterable select's keyboard highlight whenever its
input changes. Upstream kept the previous index, so after the matches shrank,
Enter read past the end of the new list, selected nothing and threw. Enter now
does nothing when no option is under the highlight, such as when nothing matches.
Upstream scrolled the highlight to the first option's height times its index,
which drifts once options differ in height or spacing; the patch scrolls just far
enough to show the highlighted option itself. Virtual lists, which mount only some
equal-height options, keep the upstream scrolling.

Only the `es/` entry consumed by Vite through the package's `module` field is
patched. HAMi-WebUI does not use the package's CommonJS or UMD builds. Dependency
installation applies the version-specific patch through pnpm; the application
image copies it before installing dependencies as well.

When upgrading TDesign, check its draft lifecycle and run the calendar browser
regressions before removing or updating this patch. Those tests cover input both
before and after popup initialization, hover restoration, cancellation, confirmed
query values, and the original responsive-calendar interaction.
