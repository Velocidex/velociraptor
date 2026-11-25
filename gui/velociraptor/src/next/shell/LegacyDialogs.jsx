// Legacy dialogs opened from the top bar. Lazy loaded so their heavy
// dependencies (markdown, syntax highlighting, tables) stay out of
// the main bundle.
import './legacy-styles.js';
import './legacy-icons.js';
export { UserSettingsModal } from '../../components/users/user-label.jsx';
export { ShowGlobalMessages } from '../../components/users/global-messages.jsx';
export { HelpDialog } from '../../components/docs/doc-button.jsx';
