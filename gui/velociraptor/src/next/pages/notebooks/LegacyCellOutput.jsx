// Charts, timelines, sigma editors and tool viewers inside notebook
// output still use the legacy renderers. Loaded lazily, only for cells
// that contain them.
import React from 'react';
import PropTypes from 'prop-types';
import '../../shell/legacy-styles.js';
import NotebookReportRenderer from '../../../components/notebooks/notebook-report-renderer.jsx';
import '../../shell/legacy-icons.js';

export default function LegacyCellOutput({ cell, notebookId, env, refresh }) {
  return (
    <div className="rx-legacy-inline rx-legacy-output">
      <NotebookReportRenderer cell={cell} notebook_id={notebookId} env={env} refresh={refresh}
        completion_reporter={() => {}} />
    </div>
  );
}

LegacyCellOutput.propTypes = {
  cell: PropTypes.object,
  notebookId: PropTypes.string,
  env: PropTypes.object,
  refresh: PropTypes.func,
};
