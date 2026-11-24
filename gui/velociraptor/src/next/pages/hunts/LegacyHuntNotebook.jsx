// The notebook tab still uses the legacy notebook renderer (cells,
// VQL editor, timelines). Loaded lazily.
import React from 'react';
import PropTypes from 'prop-types';
import '../../shell/legacy-styles.js';
import HuntNotebook from '../../../components/hunts/hunt-notebook.jsx';

export default function LegacyHuntNotebook({ hunt }) {
  return (
    <div className="rx-legacy-inline" style={{ padding: '8px 12px' }}>
      <HuntNotebook hunt={hunt} />
    </div>
  );
}

LegacyHuntNotebook.propTypes = {
  hunt: PropTypes.object,
};
