import "./navigator.css";
import _ from 'lodash';
import logo from  "./velo.svg";
import UserConfig from '../core/user.jsx';
import api from '../core/api-service.jsx';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import classNames from "classnames";
import { NavLink } from "react-router-dom";
import T from '../i8n/i8n.jsx';
import ToolTip from '../widgets/tooltip.jsx';

import { EncodePathInURL } from '../utils/paths.jsx';

class VeloNavigator extends Component {
    static contextType = UserConfig;

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
        let disabled = null;
        if (!this.props.client || !this.props.client.client_id) {
            disabled = "disabled";
        };

        let vfs_path = "";
        if (this.props.vfs_path.length) {
            vfs_path = this.props.vfs_path;
        }

        // Only show the user management screen if the user is an
        // admin in this org.
        let user_is_admin = this.context.traits &&
            this.context.traits.Permissions && (
                this.context.traits.Permissions.org_admin ||
                    this.context.traits.Permissions.server_admin);
        let customization = this.context.traits && this.context.traits.customizations;
        customization = customization || {};

        // Add sidebar links
        let sidebar_links = _.filter(
            this.context.traits ? this.context.traits.links : [],
            x=>x.type === "" || x.type === "sidebar");

        return (
            <>
              <div className="float-left navigator">
                <ToolTip tooltip={T("Expand sidebar")}>
                  <button
                    className="hamburger toolbar-buttons"
                    onClick={this.toggle}
                    aria-expanded={!this.state.collapsed}
                  >
                    <span aria-hidden="true">
                      <FontAwesomeIcon icon="bars"/>
                    </span>
                    <span className="sr-only">{T("Toggle Main Menu")}</span>
                  </button>
                </ToolTip>
                <a href="#welcome">
                  <img src={api.src_of(logo)} className="velo-logo" alt={T("Welcome")} />
                </a>
                <div
                  className={classNames({
                      collapsed: this.state.collapsed,
                      uncollapsed: !this.state.collapsed,
                  })}
                  id="navigator"
                  onClick={this.collapse}
                >
                <div>
                  <nav className="navigator" aria-labelledby="mainmenu">
                    <h2 id="mainmenu" className="sr-only">
                      {T("Main Menu")}
                    </h2>
                    <ul className="nav nav-pills navigator">
                      <li className="nav-link">
                        <NavLink exact={true} to="/dashboard">
                          <span>
                            <i className="navicon">
                              <FontAwesomeIcon icon="home" />
                            </i>
                          </span>
                          {T("Home")}
                        </NavLink>
                      </li>

                      <li className="nav-link">
                        <NavLink to="/hunts">
                          <span>
                            <i className="navicon">
                              <FontAwesomeIcon icon="crosshairs" />
                            </i>
                          </span>
                          {T("Hunt Manager")}
                        </NavLink>
                      </li>

                      <li className="nav-link">
                        <NavLink to="/artifacts">
                          <span>
                            <i className="navicon">
                              <FontAwesomeIcon icon="wrench" />
                            </i>
                          </span>
                          {T("View Artifacts")}
                        </NavLink>
                      </li>

                      {!customization.disable_server_events && (
                        <li className="nav-link">
                          <NavLink to="/events/server">
                            <span>
                              <i className="navicon">
                                <FontAwesomeIcon icon="eye" />
                              </i>
                            </span>
                            {T("Server Events")}
                          </NavLink>
                        </li>
                      )}

                      <li className="nav-link">
                        <NavLink to="/collected/server">
                          <span>
                            <i className="navicon">
                              <FontAwesomeIcon icon="server" />
                            </i>
                          </span>
                          {T("Server Artifacts")}
                        </NavLink>
                      </li>

                      <li className="nav-link">
                        <NavLink to="/notebooks">
                          <span>
                            <i className="navicon">
                              <FontAwesomeIcon icon="book" />
                            </i>
                          </span>
                          {T("Notebooks")}
                        </NavLink>
                      </li>

                      {user_is_admin && !customization.disable_user_management && (
                        <li className="nav-link">
                          <NavLink to="/users">
                            <span>
                              <i className="navicon">
                                <FontAwesomeIcon icon="user" />
                              </i>
                            </span>
                            {T("Users")}
                          </NavLink>
                        </li>
                      )}

                      <li
                        className={classNames({
                          "nav-link": true,
                          disabled: disabled,
                        })}
                      >
                        <NavLink
                          className={disabled}
                          disabled={disabled}
                          aria-hidden={disabled ? "true" : "false"}
                          tabIndex={disabled ? "-1" : "0"}
                          to={"/host/" + this.props.client.client_id}
                        >
                          <span>
                            <i className="navicon">
                              <FontAwesomeIcon icon="laptop" />{" "}
                            </i>
                          </span>
                          {T("Host Information")}
                        </NavLink>
                      </li>

                      <li
                        className={classNames({
                          "nav-link": true,
                          disabled: disabled,
                        })}
                      >
                        <NavLink
                          className={disabled}
                          disabled={disabled}
                          aria-hidden={disabled !== null ? "true" : "false"}
                          tabIndex={disabled !== null ? "-1" : "0"}
                          to={
                            "/vfs/" +
                            this.props.client.client_id +
                            EncodePathInURL(vfs_path)
                          }
                        >
                          <span>
                            <i className="navicon">
                              <FontAwesomeIcon icon="folder-open" />{" "}
                            </i>
                          </span>
                          {T("Virtual Filesystem")}
                        </NavLink>
                      </li>

                      <li
                        className={classNames({
                          "nav-link": true,
                          disabled: disabled,
                        })}
                      >
                        <NavLink
                          className={disabled}
                          disabled={disabled}
                          aria-hidden={disabled !== null ? "true" : "false"}
                          tabIndex={disabled !== null ? "-1" : "0"}
                          to={"/collected/" + this.props.client.client_id}
                        >
                          <span>
                            <i className="navicon">
                              <FontAwesomeIcon icon="history" />
                            </i>
                          </span>
                          {T("Collected Artifacts")}
                        </NavLink>
                      </li>

                      <li
                        className={classNames({
                          "nav-link": true,
                          disabled: disabled,
                        })}
                      >
                        <NavLink
                          className={disabled}
                          disabled={disabled}
                          aria-hidden={disabled !== null ? "true" : "false"}
                          tabIndex={disabled !== null ? "-1" : "0"}
                          to={"/events/" + this.props.client.client_id}
                        >
                          <span>
                            <i className="navicon">
                              <FontAwesomeIcon icon="binoculars" />
                            </i>
                          </span>
                          {T("Client Events")}
                        </NavLink>
                      </li>

                      {_.map(sidebar_links, (x) => {
                          let icon_url = x.icon_url;
                          let img = [];
                          if(icon_url) {
                              img = <img
                                      className="sidebar-icon"
                                      alt=""
                                      src={api.src_of(icon_url)}
                                    />;
                          } else {
                              img = <i className="navicon">
                                      <FontAwesomeIcon icon="external-link-alt" />
                                    </i>;
                          };

                        return (
                          <li
                            key={x.text}
                            className={classNames({
                              "nav-link": true,
                            })}
                          >
                            <a
                              href={x.url}
                              rel="noreferrer"
                              target={x.new_tab ? "_blank" : ""}
                            >
                              <ToolTip tooltip={T(x.text)}>
                                <span>{ img }</span>
                              </ToolTip>
                              {T(x.text)}
                            </a>
                          </li>
                        );
                      })}
                    </ul>
                  </nav>
                </div>
              </div>
            </div>
          </>
      );
  }
}

export default VeloNavigator;
