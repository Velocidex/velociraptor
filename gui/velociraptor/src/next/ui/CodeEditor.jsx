// Thin wrapper around the legacy Ace editor (VQL/YAML/Markdown modes,
// user preferences) so new screens can use it. Loaded lazily.
import React from 'react';
import VeloAce from '../../components/core/ace.jsx';
import '../shell/legacy-icons.js';

export default function CodeEditor({ value, onChange, mode = 'yaml', readOnly = false, minLines, maxLines, className, focus = false }) {
  return (
    <div className={'rx-legacy-inline rx-code ' + (className || '')}>
      <VeloAce
        text={value}
        mode={mode}
        focus={focus}
        onChange={onChange}
        aceConfig={ace => {
          ace.setReadOnly(!!readOnly);
          if (minLines) ace.setOption('minLines', minLines);
          if (maxLines) ace.setOption('maxLines', maxLines);
          ace.renderer.setShowPrintMargin(false);
          // Absolute line numbers always: the user's saved Ace prefs
          // (e.g. vim mode) can switch on relative numbers, which read
          // as 6,5,4...1,7 in a viewer.
          const fix = () => {
            ace.setOption('relativeLineNumbers', false);
            if (readOnly) {
              ace.setOption('highlightActiveLine', false);
              ace.setOption('highlightGutterLine', false);
              ace.renderer.$cursorLayer.element.style.display = 'none';
            }
          };
          fix();
          setTimeout(fix, 0);
        }}
      />
    </div>
  );
}
