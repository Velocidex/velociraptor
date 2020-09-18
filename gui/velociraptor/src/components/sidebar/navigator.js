import React, { Component } from 'react';
import "./navigator.css";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import classNames from "classnames";
import {
    HashRouter as Router,
    Switch,
    Route,
    NavLink
} from "react-router-dom";


class VeloNavigator extends Component {
    constructor(props) {
        super(props);
        this.state = {
            // Is the nav open or closed?
            collapsed: true,

            // client_id we are currently viewing
            client_id: props.client_id,
        };

        this.toggle = this.toggle.bind(this);
        this.collapse = this.collapse.bind(this);
    };

    toggle() {
        let new_state  = Object.assign({}, this.state);
        new_state.collapsed = !new_state.collapsed;
        this.setState(new_state);
    };

    collapse() {
        let new_state  = Object.assign({}, this.state);
        new_state.collapsed = true;
        this.setState(new_state);
    };

    render() {
        return (
            <Router>
              <div className="float-left navigator">
                <span className="hamburger toolbar-buttons"
                      onClick={this.toggle}>&#9776;</span>
                <a href="#welcome">
                  <img src="/static/images/velo.svg" className="velo-logo"/>
                </a>
                <div className={classNames({
                    'collapsed': this.state.collapsed,
                    'uncollapsed': !this.state.collapsed})}
                     id="navigator"
                     onClick={this.collapse}>
                  <div>
                    <section className="navigator">
                      <NavLink exact={true} to="/">
                        <ul className="nav nav-pills navigator">
                          <li grr-nav-link className="nav-link" state="userDashboard">
                            <i className="navicon"><FontAwesomeIcon icon="home"/></i>
                            Home
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink exact={true}  to="/hunts">
                        <ul className="nav nav-pills navigator">
                          <li grr-nav-link className="nav-link" state="hunts" >
                            <i className="navicon"><FontAwesomeIcon icon="crosshairs"/></i>
                            Hunt Manager
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink exact={true} to="/artifacts">
                        <ul className="nav nav-pills navigator">
                          <li grr-nav-link className="nav-link" state="view_artifacts" >
                            <i className="navicon"><FontAwesomeIcon icon="wrench"/></i>
                            View Artifacts
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink exact={true}  to="/server_events">
                        <ul className="nav nav-pills  navigator">
                          <li grr-nav-link className="nav-link" state="server_events" >
                            <i className="navicon"><FontAwesomeIcon icon="eye"/></i>
                            Server Events
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink exact={true}  to="/server_artifacts">
                        <ul className="nav nav-pills  navigator">
                          <li grr-nav-link className="nav-link" state="server_artifacts" >
                            <i className="navicon"><FontAwesomeIcon icon="server"/></i>
                            Server Artifacts
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink exact={true}  to="/notebooks">
                        <ul className="nav nav-pills navigator">
                          <li grr-nav-link className="nav-link" state="notebook" >
                            <i className="navicon"><FontAwesomeIcon icon="book"/></i>
                            Notebooks
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink exact={true}  to="/host">
                        <ul className="nav nav-pills navigator">
                          <li className={classNames({
                              "nav-link": true,
                              disabled: !this.state.client_id})}>
                            <i className="navicon"><FontAwesomeIcon icon="laptop"/> </i>
                            Host Information
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink exact={true}  to="/vfs">
                        <ul className="nav nav-pills navigator">
                          <li className={classNames({
                              "nav-link": true,
                              disabled: !this.state.client_id})}>
                            <i className="navicon"><FontAwesomeIcon icon="folder-open"/> </i>
                            Virtual Filesystem
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink exact={true}  to="/collected">
                        <ul className="nav nav-pills navigator">
                          <li className={classNames({
                              "nav-link": true,
                              disabled: !this.state.client_id})}>
                            <i className="navicon"><FontAwesomeIcon icon="history"/></i>
                            Collected Artifacts
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink exact={true}  to="/client_events">
                        <ul className="nav nav-pills navigator">
                          <li className={classNames({
                              "nav-link": true,
                              disabled: !this.state.client_id})}>
                            <i className="navicon"><FontAwesomeIcon icon="binoculars"/></i>
                            Client Events
                          </li>
                        </ul>
                      </NavLink>

                    </section>
                  </div>
                </div>

              </div>
            </Router>
        );
    }
}

export default VeloNavigator;
