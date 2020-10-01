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
import Notebook from './components/notebooks/notebook.js';

import { Switch, Route } from "react-router-dom";
import { Join } from './components/utils/paths.js';

import Navbar from 'react-bootstrap/Navbar';
import Nav from 'react-bootstrap/Nav';


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
    }

    // Called to update the client id.
    setClient = (client) => {
        this.setState({client: client});
    };

    setClientSearch = (query) => {
        this.setState({query: query});
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
            <>
              <div className="navbar navbar-default navbar-static-top" id="header">
                <div className="navbar-inner">
                  <div className="">
                    <VeloNavigator
                      vfs_path={vfs_path}
                      client={this.state.client} />
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
                  <Route path="/vfs/:client_id/:vfs_path(.*)">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                    <VFSViewer client={this.state.client}
                               selectedRow={this.state.selected_row}
                               setSelectedRow={this.setSelectedRow}
                               updateVFSPath={this.updateVFSPath}
                               updateCurrentNode={this.updateCurrentNode}
                               node={this.state.current_node}
                               vfs_path={this.state.vfs_path} />
                  </Route>
                  <Route path="/collected/:client_id/:flow_id?/:tab?">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                    <ClientFlowsView
                      client={this.state.client}
                    />
                  </Route>
                  <Route path="/notebooks/:notebook_id?">
                    <Notebook />
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
