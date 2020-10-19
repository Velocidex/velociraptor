import React, { Component } from 'react';
import './App.css';

import VeloNavigator from './components/sidebar/navigator.js';
import VeloClientSearch from './components/clients/search.js';
import VeloClientList from './components/clients/clients-list.js';
import VeloHostInfo from './components/clients/host-info.js';
import ClientSetterFromRoute from './components/clients/client_info.js';
import VeloClientSummary from './components/clients/client-summary.js';
import VFSViewer from './components/vfs/browse-vfs.js';
import VeloLiveClock from './components/utils/clock.js';
import ClientFlowsView from './components/flows/client-flows-view.js';
import ServerFlowsView from './components/flows/server-flows-view.js';
import Notebook from './components/notebooks/notebook.js';
import ArtifactInspector from './components/artifacts/artifacts.js';
import VeloHunts from './components/hunts/hunts.js';
import UserDashboard from './components/sidebar/user-dashboard.js';
import Form from 'react-bootstrap/Form';
import UserLabel from './components/users/user-label.js';
import EventMonitoring from './components/events/events.js';
import SnackbarProvider from 'react-simple-snackbar';
import Snackbar from './components/core/snackbar.js';
import Welcome from './components/welcome/welcome.js';

import { UserSettings } from './components/core/user.js';

import { Switch, Route } from "react-router-dom";
import { Join } from './components/utils/paths.js';

import Navbar from 'react-bootstrap/Navbar';
import Nav from 'react-bootstrap/Nav';

import SidebarKeyNavigator from './components/sidebar/hotkeys.js';

/* This is the main App page.

  We maintain internal state:

  client - the currently selected client the user will interact
     with. We hold the entire client info object in the top level
     state.

  query - the current search query typed into the top toolbar - this
     is always exposed and only changed by the user. It allows us to
     re-search for the same term quickly.

  The client information is populated via the setClient() callback by:
  1. VeloClientList component receiving a click on a table row.

*/

class App extends Component {
    state = {
        client: {},

        // The file listing in the current directory.
        current_node: {},

        // The current row within the current node that is selected.
        selected_row: {},

        query: "",
        query_version: "",
    }

    // Called to update the current client.
    setClient = (client) => {
        this.setState({client: client});
    };

    setClientSearch = (query) => {
        let now = new Date();
        this.setState({query: query, query_version: now.getTime()});
    };

    updateCurrentNode = (node) => {
        this.setState({current_node: node});
    }

    render() {
        // We need to prepare a vfs_path for the navigator to link
        // to. Depending on the current node, we make a link with or
        // without a final "/".
        let vfs_path = "/";
        if (this.state.current_node && this.state.current_node.path) {
            let path = [...this.state.current_node.path];
            let name = this.state.current_node.selected && this.state.current_node.selected.Name;
            if (name) {
                path.push(name);
                vfs_path = Join(path);
            } else {
                vfs_path = Join(path) + "/";
            }
        }

        return (
            <UserSettings><SnackbarProvider><SidebarKeyNavigator client={this.state.client}/>
              <Navbar fixed="top" className="main-navbar justify-content-between">
                <Form inline>
                  <VeloNavigator
                    vfs_path={vfs_path}
                    client={this.state.client} />

                  <VeloClientSearch
                    setSearch={this.setClientSearch}
                  />
                </Form>
                <VeloClientSummary
                  setClient={this.setClient}
                  client={this.state.client}/>
                <UserLabel className="navbar-text"/>
              </Navbar>
              <div id="content">
                <Switch>
                  <Route path="/dashboard">
                    <UserDashboard />
                  </Route>
                  <Route exact path="/" component={Welcome}/>
                  <Route exact path="/welcome" component={Welcome}/>
                  <Route path="/search/:query?">
                    <VeloClientList
                      setSearch={this.setClientSearch}
                      version={this.state.query_version}
                      query={this.state.query}
                      setClient={this.setClient}
                    />
                  </Route>
                  <Route path="/artifacts/:artifact?">
                    <ArtifactInspector/>
                  </Route>
                  <Route path="/hunts/:hunt_id?/:tab?">
                    <VeloHunts/>
                  </Route>

                  <Route path="/host/:client_id/:action?">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                    <VeloHostInfo client={this.state.client}  />
                  </Route>
                  <Route path="/vfs/:client_id/:vfs_path(.*)">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                    <VFSViewer client={this.state.client}
                               selectedRow={this.state.selected_row}
                               updateCurrentNode={this.updateCurrentNode}
                               node={this.state.current_node}
                               vfs_path={this.state.vfs_path} />
                  </Route>
                  {/* ClientFlowsView will only be invoked when the
                    * client is starts with C - the ServerFlowsView is
                    * invoked when client_id == "server"
                    */}
                  <Route path="/collected/:client_id(C[^/]+)/:flow_id?/:tab?">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                    <ClientFlowsView client={this.state.client} />
                  </Route>
                  <Route path="/collected/server/:flow_id?/:tab?">
                    <ServerFlowsView />
                  </Route>
                  <Route path="/notebooks/:notebook_id?">
                    <Notebook />
                  </Route>
                  <Route path="/events/:client_id(C[^/]+)/:artifact?/:time?">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                    <EventMonitoring client={this.state.client}/>
                  </Route>
                  <Route path="/events/server/:artifact?/:time?">
                    <EventMonitoring client={{client_id: ""}}/>
                  </Route>
                </Switch>
              </div>
              <Navbar fixed="bottom" className="app-footer justify-content-between ">
                <Nav></Nav>
                <Nav>
                  <VeloLiveClock className="float-right" />
                  <Snackbar />
                </Nav>
              </Navbar>
            </SnackbarProvider></UserSettings>
        );
    };
}

export default App;
