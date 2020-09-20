import React, { Component } from 'react';
import './App.css';

import VeloNavigator from './components/sidebar/navigator.js';
import VeloClientSearch from './components/clients/search.js';
import VeloClientList from './components/clients/clients-list.js';
import VeloHostInfo from './components/clients/host-info.js';
import ClientSetterFromRoute from './components/clients/client_info.js';
import VeloClientSummary from './components/clients/client-summary.js';

import { Switch, Route } from "react-router-dom";


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
        query: "",
    }

    constructor() {
        super();

        this.setClient = this.setClient.bind(this);
        this.setClientSearch = this.setClientSearch.bind(this);
    }

    // Called to update the client id.
    setClient(client) {
        let new_state  = Object.assign({}, this.state);
        new_state.client = client;
        this.setState(new_state);
    };

    setClientSearch(query) {
        let new_state  = Object.assign({}, this.state);
        new_state.query = query;
        this.setState(new_state);
    };

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
                  <Route path="/vfs/:client_id">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                  </Route>
                  <Route path="/collected/:client_id">
                    <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                  </Route>
                </Switch>
              </div>
              <div className="navbar navbar-default navbar-fixed-bottom" id="footer">
                <div className="navbar-inner">
                  <a className="brand"></a>
                  <ul className="nav navbar-nav float-left">
                    <li>
                      <grr-server-error-preview></grr-server-error-preview>
                    </li>
                  </ul>
                  <ul className="nav navbar-nav float-right">
                    <li >
                      <grr-live-clock className="hidden-xs"></grr-live-clock>
                    </li>
                  </ul>
                </div>
              </div>

            </>
        );
    };
}

export default App;
