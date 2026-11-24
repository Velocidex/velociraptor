// Thin wrapper around the legacy Ace editor (VQL/YAML/Markdown modes,
// user preferences) so new screens can use it. Loaded lazily.
import React from 'react';
import VeloAce from '../../components/core/ace.jsx';

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
          if (readOnly) ace.renderer.$cursorLayer.element.style.display = 'none';
        }}
      />
    </div>
  );
}
