import "./navigator.css";
import logo from  "./velo.svg";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import classNames from "classnames";
import { NavLink } from "react-router-dom";

class VeloNavigator extends Component {
    static propTypes = {
        client: PropTypes.object.isRequired,
        vfs_path: PropTypes.string,
    }

    constructor(props) {
        super(props);
        this.state = {
            // Is the nav open or closed?
            collapsed: true,
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
        let disabled = "";
        if (!this.props.client || !this.props.client.client_id) {
            disabled = "disabled";
        };

        let vfs_path = "";
        if (this.props.vfs_path.length) {
            vfs_path = this.props.vfs_path;
        }

        return (
            <>
              <div className="float-left navigator">
                <span className="hamburger toolbar-buttons"
                      onClick={this.toggle}>&#9776;</span>
                <a href="#welcome">
                  <img src={logo} className="velo-logo" alt="velo logo"/>
                </a>
                <div className={classNames({
                    'collapsed': this.state.collapsed,
                    'uncollapsed': !this.state.collapsed})}
                     id="navigator"
                     onClick={this.collapse}>
                  <div>
                    <section className="navigator">
                      <NavLink exact={true} to="/dashboard">
                        <ul className="nav nav-pills navigator">
                          <li className="nav-link" state="userDashboard">
                            <i className="navicon"><FontAwesomeIcon icon="home"/></i>
                            Home
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink to="/hunts">
                        <ul className="nav nav-pills navigator">
                          <li className="nav-link" state="hunts" >
                            <i className="navicon"><FontAwesomeIcon icon="crosshairs"/></i>
                            Hunt Manager
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink to="/artifacts">
                        <ul className="nav nav-pills navigator">
                          <li className="nav-link" state="view_artifacts" >
                            <i className="navicon"><FontAwesomeIcon icon="wrench"/></i>
                            View Artifacts
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink to="/events/server">
                        <ul className="nav nav-pills  navigator">
                          <li className="nav-link" state="server_events" >
                            <i className="navicon"><FontAwesomeIcon icon="eye"/></i>
                            Server Events
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink to="/collected/server">
                        <ul className="nav nav-pills  navigator">
                          <li className="nav-link" state="server_artifacts" >
                            <i className="navicon"><FontAwesomeIcon icon="server"/></i>
                            Server Artifacts
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink to="/notebooks">
                        <ul className="nav nav-pills navigator">
                          <li className="nav-link" state="notebook" >
                            <i className="navicon"><FontAwesomeIcon icon="book"/></i>
                            Notebooks
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink className={disabled}
                               to={"/host/" + this.props.client.client_id}>
                        <ul className="nav nav-pills navigator">
                          <li className={classNames({
                              "nav-link": true,
                              disabled: disabled})}>
                            <i className="navicon"><FontAwesomeIcon icon="laptop"/> </i>
                            Host Information
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink className={disabled}
                               to={"/vfs/" + this.props.client.client_id + vfs_path }>
                        <ul className="nav nav-pills navigator">
                          <li className={classNames({
                              "nav-link": true,
                              disabled: disabled})}>
                            <i className="navicon"><FontAwesomeIcon icon="folder-open"/> </i>
                            Virtual Filesystem
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink className={disabled}
                               to={"/collected/" + this.props.client.client_id}>
                        <ul className="nav nav-pills navigator">
                          <li className={classNames({
                              "nav-link": true,
                              disabled: disabled})}>
                            <i className="navicon"><FontAwesomeIcon icon="history"/></i>
                            Collected Artifacts
                          </li>
                        </ul>
                      </NavLink>

                      <NavLink className={disabled}
                               to={"/events/" + this.props.client.client_id}>
                        <ul className="nav nav-pills navigator">
                          <li className={classNames({
                              "nav-link": true,
                              disabled: disabled})}>
                            <i className="navicon"><FontAwesomeIcon icon="binoculars"/></i>
                            Client Events
                          </li>
                        </ul>
                      </NavLink>

                    </section>
                  </div>
                </div>
              </div>
            </>
        );
    }
}

export default VeloNavigator;
