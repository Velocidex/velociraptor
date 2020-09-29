import React, { Component } from 'react';
import './App.css';

import VeloNavigator from './components/sidebar/navigator.js';
import VeloClientSearch from './components/clients/search.js';
import VeloClientList from './components/clients/clients-list.js';
import VeloHostInfo from './components/clients/host-info.js';
import ClientSetterFromRoute from './components/clients/client_info.js';
import VeloClientSummary from './components/clients/client-summary.js';
import VFSViewer from './components/vfs/browse-vfs.js';
import VFSSetterFromRoute from './components/vfs/vfs-setter.js';
import VeloLiveClock from './components/utils/clock.js';
import ClientFlowsView from './components/flows/client-flows-view.js';
import { Switch, Route } from "react-router-dom";

import Navbar from 'react-bootstrap/Navbar';
import Nav from 'react-bootstrap/Nav';


/* This is the main App page.

  We maintain internal state:

  * client - the currently selected client the user will interact
  * with. We hold the entire client info object in the top level
  * state.

  * query - the current search query typed into the top toolbar - this
  *    is always exposed and only changed by the user. It allows us to
  *    re-search for the same term quickly.

  The client information is populated via the setClient() callback by:
  1. VeloClientList component receiving a click on a table row.

*/

class App extends Component {
    state = {
        client: {},

        // Current path components to the VFS directory.
        vfs_path: [],

        // The file listing in the current directory.
        current_node: {},

        // The current row within the current node that is selected.
        selected_row: {},

        query: "",
    }

    // Called to update the client id.
    setClient = (client) => {
        let new_state  = Object.assign({}, this.state);
        new_state.client = client;
        this.setState(new_state);
    };

    setClientSearch = (query) => {
        let new_state  = Object.assign({}, this.state);
        new_state.query = query;
        this.setState(new_state);
    };

    setSelectedRow = (row) => {
        this.setState({selected_row: row});
    };

    updateVFSPath = (vfs_path) => {
        this.setState({vfs_path: vfs_path});
    };

    updateCurrentNode = (node) => {
        this.setState({current_node: node});
    }

    render() {
        return (
            <>
              <div className="navbar navbar-default navbar-static-top" id="header">
                <div className="navbar-inner">
                  <div className="">
                    <VeloNavigator client={this.state.client} />
                    <div className="float-left navbar-form toolbar-buttons">
                      <VeloClientSearch
                        setSearch={this.setClientSearch}
                      />
                    </div>
                    <VeloClientSummary client={this.state.client}/>
                    <div className="nav float-right">
                      <grr-user-label className="navbar-text"></grr-user-label>
                    </div>
                    <div className="nav float-right toolbar-buttons">
                    </div>
                  </div>
                </div>
              </div>
              <div id="content">
                <Switch>
                  <Route path="/search">
                    <VeloClientList query={this.state.query}
                                    setClient={this.setClient}
                    />
                  </Route>
                  <Route path="/host/:client_id/:action?">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                    <VeloHostInfo client={this.state.client}  />
                  </Route>
                  <Route path="/vfs/:client_id/:vfs_path*">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                    <VFSSetterFromRoute vfs_path={this.state.vfs_path}
                                        updateVFSPath={this.updateVFSPath} />
                    <VFSViewer client={this.state.client}
                               selectedRow={this.state.selected_row}
                               setSelectedRow={this.setSelectedRow}
                               updateVFSPath={this.updateVFSPath}
                               updateCurrentNode={this.updateCurrentNode}
                               node={this.state.current_node}
                               vfs_path={this.state.vfs_path} />
                  </Route>
                  <Route path="/collected/:client_id">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                    <ClientFlowsView
                      client={this.state.client}
                    />
                  </Route>
                </Switch>
              </div>
              <Navbar fixed="bottom">
                <Nav className="col-9">
                </Nav>
                <Nav>
                  <VeloLiveClock className="col-3 float-right" />
                </Nav>
              </Navbar>

            </>
        );
    };
}

export default App;
