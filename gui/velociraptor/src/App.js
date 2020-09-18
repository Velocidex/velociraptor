import React, { Component } from 'react';

import logo from './logo.svg';
import './App.css';

import api from './components/core/api-service.js';
import VeloNavigator from './components/sidebar/navigator.js';
import VeloClientSearch from './components/clients/search.js';

import Navbar from 'react-bootstrap/Navbar';
import Nav from 'react-bootstrap/Nav';
import NavDropdown from 'react-bootstrap/NavDropdown';
import Form from 'react-bootstrap/Form';
import FormControl from 'react-bootstrap/FormControl';
import Button from 'react-bootstrap/Button';

import {
    HashRouter as Router,
    Switch,
    Route,
    Link
} from "react-router-dom";

class App extends Component {
    constructor() {
        super();

        // Global application state.
        this.state = {
            users: "",

            // Currently viewing client
            client_id: null,
        };
        this.getUsers = this.getUsers.bind(this);

        this.setClient = this.setClient.bind(this);
    }

    // Called to update the client id.
    setClient(client_id) {
        let new_state  = Object.assign({}, this.state);
        new_state.client_id = client_id;
        this.setState(new_state);
    };

    componentDidMount() {
        this.getUsers();
    }

    async getUsers() {
        api.get("api/v1/SearchClients").then(response => {
            this.setState({users: JSON.stringify(response)});
        });
    };

    render() {
        return (
            <>
              <div className="navbar navbar-default navbar-static-top" id="header">
                <div className="navbar-inner">
                  <div className="">
                    <VeloNavigator clientId={this.state.client_id} />
                    <div className="float-left navbar-form toolbar-buttons">
                      <VeloClientSearch setClientId={this.setClientId}/>
                    </div>
                    <grr-client-summary></grr-client-summary>
                    <div className="nav float-right">
                      <grr-user-label className="navbar-text"></grr-user-label>
                    </div>
                    <div className="nav float-right toolbar-buttons">
                    </div>
                  </div>
                </div>
              </div>
              <div id="content">
                <Router>
                  <Switch>
                    <Route path="/about">
                      This is the about page
                    </Route>
                    <Route path="/users">
                      This is the users page
                    </Route>
                    <Route path="/">
                      This is the main page
                    </Route>
                  </Switch>
                </Router>
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
