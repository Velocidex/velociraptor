// Screens that have not been rebuilt yet. They render unchanged inside
// the new shell. This module is loaded lazily so the new screens do
// not pay for the legacy code (notebooks, editors, VFS ...).
import React, { useState } from 'react';
import PropTypes from 'prop-types';
import { Switch, Route } from 'react-router-dom';

import './legacy-styles.js';

import ClientSetterFromRoute from '../../components/clients/client_info.jsx';
import VeloHostInfo from '../../components/clients/host-info.jsx';
import ServerInfo from '../../components/server/server-info.jsx';
import VFSViewer from '../../components/vfs/browse-vfs.jsx';
import ClientFlowsView from '../../components/flows/client-flows-view.jsx';
import ServerFlowsView from '../../components/flows/server-flows-view.jsx';
import Notebook from '../../components/notebooks/notebook.jsx';
import ArtifactInspector from '../../components/artifacts/artifacts.jsx';
import UserInspector from '../../components/users/user-inspector.jsx';
import EventMonitoring from '../../components/events/events.jsx';
import SecretManager from '../../components/secrets/secrets.jsx';
import VeloHunts from '../../components/hunts/hunts.jsx';
import UserDashboard from '../../components/sidebar/user-dashboard.jsx';
import VeloClientList from '../../components/clients/clients-list.jsx';
import './legacy-icons.js';

export default function LegacyRoutes({ client, setClient }) {
  const [currentNode, setCurrentNode] = useState({});

  return (
    <div className="rx-legacy-host">
      <div id="content">
        <Switch>
          {/* The classic screens stay reachable while the new ones mature. */}
          <Route path="/classic/hunts/:hunt_id?/:tab?/:params_json?">
            <VeloHunts />
          </Route>
          <Route path="/classic/dashboard">
            <UserDashboard />
          </Route>
          <Route path="/classic/search/:query?">
            <VeloClientList setSearch={() => undefined} version={0} query="" setClient={setClient} />
          </Route>
          <Route path="/classic/secrets/">
            <SecretManager />
          </Route>
          <Route path="/classic/artifacts/:artifact?/:action?">
            <ArtifactInspector client={client} />
          </Route>
          <Route path="/classic/users/:user?" component={UserInspector} />
          <Route path="/classic/host/:client_id([^/]{7,})/:action?">
            <ClientSetterFromRoute client={client} setClient={setClient} />
            <VeloHostInfo client={client} setClient={setClient} />
          </Route>
          <Route path="/host/:client_id(server)/:action?">
            <ServerInfo />
          </Route>
          <Route path="/classic/vfs/:client_id/:vfs_path(.*)">
            <ClientSetterFromRoute client={client} setClient={setClient} />
            <VFSViewer client={client}
              selectedRow={{}}
              updateCurrentNode={setCurrentNode}
              node={currentNode}
              vfs_path={undefined} />
          </Route>
          <Route path="/classic/collected/:client_id([^/]{7,})/:flow_id?/:tab?/:params_json?">
            <ClientSetterFromRoute client={client} setClient={setClient} />
            <ClientFlowsView client={client} />
          </Route>
          <Route path="/classic/collected/server/:flow_id?/:tab?">
            <ServerFlowsView />
          </Route>
          <Route path={["/classic/notebooks/:notebook_id?/:artifact?/:params_json?", "/notebooks/:notebook_id/:artifact/:params_json?"]}>
            <Notebook />
          </Route>
          <Route path="/classic/events/:client_id([^/]{7,})/:artifact?/:time?">
            <ClientSetterFromRoute client={client} setClient={setClient} />
            <EventMonitoring client={client} />
          </Route>
          <Route path="/classic/events/server/:artifact?/:time?">
            <EventMonitoring client={{ client_id: '' }} />
          </Route>
          <Route>
            <div className="no-content">Page not found</div>
          </Route>
        </Switch>
      </div>
    </div>
  );
}

LegacyRoutes.propTypes = {
  client: PropTypes.object,
  setClient: PropTypes.func,
};
