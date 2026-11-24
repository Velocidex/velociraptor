// Full screen legacy views (notebooks, shell, tables). Lazy loaded.
import React from 'react';
import { Switch, Route } from 'react-router-dom';
import './legacy-styles.js';
import FullScreenTable from '../../components/notebooks/table_view.jsx';
import FullScreenNotebook from '../../components/notebooks/full_notebook.jsx';
import FullScreenHuntNotebook from '../../components/hunts/hunt-full-notebook.jsx';
import FullScreenFlowNotebook from '../../components/flows/flow-full-notebook.jsx';
import { ShellViewerFullScreen } from '../../components/clients/shell-viewer.jsx';

export default function LegacyFullscreen() {
  return (
    <Switch>
      <Route path="/fullscreen/shell/:artifact/:client_id/:flow_id">
        <ShellViewerFullScreen />
      </Route>
      <Route path="/fullscreen/notebooks/:notebook_id">
        <FullScreenNotebook />
      </Route>
      <Route path="/fullscreen/hunts/:hunt_id/notebook">
        <FullScreenHuntNotebook />
      </Route>
      <Route path="/fullscreen/collected/:client_id/:flow_id/notebook">
        <FullScreenFlowNotebook />
      </Route>
      <Route path="/fullscreen/table/:state">
        <FullScreenTable />
      </Route>
    </Switch>
  );
}
