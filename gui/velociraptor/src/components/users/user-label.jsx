import './user-label.css';
import _ from 'lodash';
import moment from 'moment';
import 'moment-timezone';
import qs from "qs";

import React from 'react';
import PropTypes from 'prop-types';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter }  from "react-router-dom";

import Accordion from 'react-bootstrap/Accordion';
import UserConfig from '../core/user.jsx';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import T from '../i8n/i8n.jsx';
import InputGroup from 'react-bootstrap/InputGroup';
import Select from 'react-select';
import ToolTip from '../widgets/tooltip.jsx';

import { JSONparse } from '../utils/json_parse.jsx';

class _PasswordChange extends React.Component {
    static propTypes = {
        username: PropTypes.string,
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    updatePassword = ()=>{
        api.post("v1/SetPassword", {
            username: this.props.username,
            password: this.state.password1,
        }, this.source.token).then((response) => {
            this.props.onClose();
        });
    }

    state = {
        password1: "",
        password2: "",
    }

    render() {
        return  (
            <>
              <Form.Control type="password"
                            value={this.state.password1}
                            onChange={e=>this.setState({
                                password1: e.currentTarget.value})}
                            placeholder={T("Password")} />
              <Form.Control type="password"
                            value={this.state.password2}
                            onChange={e=>this.setState({
                                password2: e.currentTarget.value})}
                            placeholder={T("Retype Password")} />
              { this.state.password1 && this.state.password2 &&
                <Button variant={
                    this.state.password1 !== this.state.password2 ? "warning" : "default"}
                        size="sm"
                        className="set-password-button"
                        disabled={this.state.password1 !== this.state.password2}
                        onClick={this.updatePassword}
                >
                  {this.state.password1 !== this.state.password2 ? T("Passwords do not match") : T("Submit")}
                </Button>
              }
            </>);
    }
}

export const PasswordChange = withRouter(_PasswordChange);

class _PasswordChangeForm extends React.PureComponent {
    static propTypes = {
        username: PropTypes.string,
        onClose: PropTypes.func.isRequired,

        // React router props.
        history: PropTypes.object,
    }

    render() {
        return  (
            <Form.Group as={Row}>
              <Form.Label column sm="3">
                {T("Password")}
              </Form.Label>
              <Col sm="8">
                <Accordion>
                  <Accordion.Item eventKey="0">
                    <Accordion.Header>
                      {T("Update Password")}
                    </Accordion.Header>
                    <Accordion.Body>
                      <PasswordChange
                        username={this.props.username}
                        onClose={()=>{
                            // Redirect to the homepage so as to get a
                            // good state
                            this.props.history.push("/welcome");
                            this.props.onClose();
                        }}/>
                    </Accordion.Body>
                  </Accordion.Item>
                </Accordion>
              </Col>
            </Form.Group>
        );
    }
}

export const PasswordChangeForm = withRouter(_PasswordChangeForm);

class UserSettingsDialog extends React.PureComponent {
    static contextType = UserConfig;
    static propTypes = {
        onClose: PropTypes.func.isRequired,
        setSetting: PropTypes.func.isRequired,
        setClient: PropTypes.func.isRequired,
        history: PropTypes.object,
    }

    componentDidMount = () => {
        if (this.context.traits) {
            this.setState({
                theme: this.context.traits.theme || "veloci-light",
                timezone: this.context.traits.timezone || "UTC",
                lang: this.context.traits.lang || "en",
                org: window.globals.OrgId,
                default_password: this.context.traits.default_password || "",
            });
        }
    }

    state = {
        theme: "",
        timezone: "",
        default_password: "",
        org: "",
        org_changed: false,
        edited: false
    }

    saveSettings = (settings)=> {
        // Only update the settings that changed.
        let state = Object.assign({}, this.state);
        if(settings) {
            state = Object.assign(state, settings);
        }

        let params = {};
        if (state.previous_org) {
            params.previous_org = state.previous_org;
        }

        if (state.theme !== this.context.traits.theme) {
            params.theme = state.theme;
        }

        if (state.timezone !== this.context.traits.timezone) {
            params.timezone = state.timezone;
        }

        if (state.lang !== this.context.traits.lang) {
            params.lang = state.lang;
        }

        if (state.org_changed) {
            params.org = state.org;
        }

        if (state.default_password !== (this.context.traits.default_password || "")) {
            params.default_password = state.default_password || "-";
        }

        // Update the base path if needed.
        if (this.context.traits.base_path != "" &&
            this.context.traits.base_path !== window.base_path) {
            window.base_path = this.context.traits.base_path;
        }

        this.props.setSetting(params);
    }

    changeOrg = org=>{
        // Set the URL to the required org_id
        let search = window.location.search.replace('?', '');
        let params = qs.parse(search);
        params.org_id = org;

        // Revert to this org on error.
        let previous_org = window.globals.OrgId;

        window.globals.OrgId = org;

        // Force navigation to the welcome screen to make sure the GUI
        // is reset.
        window.history.pushState({}, "", api.href("/app/index.html", params));
        this.props.history.replace("/welcome");

        // Set the new settings for the next reload. This will be the
        // default org id next time the user loads up the main page.
        this.saveSettings({org: org,
                           org_changed: true,
                           previous_org: previous_org});
        this.props.setClient({client_id: null});
        this.props.onClose();
    }

    render() {
        return (
            <Modal show={true}
                   dialogClassName="modal-70w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("User Settings")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                { this.context.traits.orgs &&
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">
                      <ToolTip tooltip={T("Switch to a different org")}>
                        <div>{T("Organization")}</div>
                      </ToolTip>
                    </Form.Label>
                    <Col sm="8">
                      <Select
                        value={this.state.org}
                        placeholder={T("Select an org")}
                        classNamePrefix="velo"
                        onChange={e=>this.changeOrg(e.value)}
                        spellCheck="false"
                        options={_.map(_.sortBy(
                            this.context.traits.orgs, x=>x.name) || [],
                                       k=>{
                                           return {value: k.id,
                                                   label: k.name,
                                                   isFixed: true};
                                       })}
                      />
                    </Col>
                  </Form.Group>
                }
                { !this.context.traits.password_less &&
                  <PasswordChangeForm
                    onClose={this.props.onClose}
                    >
                  </PasswordChangeForm> }
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    {T("Theme")}
                  </Form.Label>
                  <Col sm="8">
                    <Form.Control as="select"
                                  value={this.state.theme}
                                  placeholder={T("Select a theme")}
                                  onChange={(e) => {
                                      this.setState({theme: e.currentTarget.value});

                                      // Change the theme instantly
                                      this.props.setSetting({
                                          theme: e.currentTarget.value,
                                      });
                                  }}>
                      <option value="veloci-light">{T("Velociraptor (light)")}</option>
                      <option value="veloci-dark">{T("Velociraptor (dark)")}</option>
                      <option value="vscode-dark">{T("VS Code (dark)")}</option>
                      <option value="no-theme">{T("Velociraptor Classic (light)")}</option>
                      <option value="pink-light">{T("Strawberry Milkshake (light)")}</option>
                      <option value="ncurses-light">{T("Ncurses (light)")}</option>
                      <option value="ncurses-dark">{T("Ncurses (dark)")}</option>
                      {/* <option value="github-light">{T("Github (light)")}</option> */}
                      <option value="github-dimmed-dark">{T("Github dimmed (dark)")}</option>
                      <option value="coolgray-dark">{T("Cool Gray (dark)")}</option>
                      <option value="midnight">{T("Midnight Inferno (very dark)")}</option>
                      <option value="veloci-docs">{T("Standard Docs")}</option>
                    </Form.Control>
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <ToolTip tooltip={T("Default password to use for downloads")}>
                      <div>{T("Downloads Password")}</div>
                    </ToolTip>
                  </Form.Label>
                  <Col sm="8">
                    <InputGroup className="mb-3">
                      <Form.Control
                        type="text"
                        value={this.state.default_password}
                        placeholder={T("Default password to use for downloads")}
                        onChange={e => {this.setState({"edited": true});
                                          this.setState({
                                              default_password: e.currentTarget.value
                                          });
                                         }}/>
                      <Button as="button" variant="default"
                              disabled = {!this.state.edited}
                              onClick={e => {
                                  this.props.setSetting({
                                      default_password: this.state.default_password
                                  });
                                  this.setState({
                                      edited: false,
                                  });
                              }}>
                        <FontAwesomeIcon icon="save" />
                      </Button>
                      <Button variant="default"
                              disabled={!this.state.default_password}
                              onClick={()=>{
                                  this.props.setSetting({
                                      default_password: "",
                                  });
                                  this.setState({
                                      default_password: "",
                                      edited: false});
                              }}>
                        <FontAwesomeIcon icon="broom"/>
                      </Button>
                    </InputGroup></Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <ToolTip tooltip={T("Select a language")}>
                      <div>{T("Language")}</div>
                    </ToolTip>
                  </Form.Label>
                  <Col sm="8">
                    <Form.Control as="select"
                                  value={this.state.lang}
                                  placeholder={T("Select a language")}
                                  onChange={(e) => {
                                      this.setState({lang: e.currentTarget.value});

                                      // Change the language instantly
                                      // (We might still need to wait
                                      // for a react render event).
                                      this.saveSettings({
                                          lang: e.currentTarget.value,
                                      });
                                  }}>
                      <option value="en">{T("English")}</option>
                      <option value="de">{T("Deutsch")}</option>
                      <option value="es">{T("Spanish")}</option>
                      <option value="por">{T("Portuguese")}</option>
                      <option value="fr">{T("French")}</option>
                      <option value="jp">{T("Japanese")}</option>
                      <option value="vi">{T("Vietnamese")}</option>
                    </Form.Control>
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <ToolTip tooltip={T("Select a timezone")}>
                      <div>{T("Display timezone")}</div>
                    </ToolTip>
                  </Form.Label>
                  <Col sm="8">
                    <InputGroup className="mb-3">
                        <Button
                          className="btn btn-default"
                          onClick={()=>{
                              this.setState({timezone: "UTC"});
                              this.props.setSetting({
                                  timezone: "UTC",
                              });
                          }}>
                          UTC
                        </Button>
                      <Select
                        className="timezone-selector"
                        classNamePrefix="velo"
                        placeholder={this.state.timezone}
                        onChange={(e)=>{
                            this.setState({timezone: e.value});

                            // Change the language instantly
                            // (We might still need to wait
                            // for a react render event).
                            this.props.setSetting({
                                timezone: e.value,
                            });
                        }}
                        options={_.map(moment.tz.names(), x=>{
                             return {value: x, label: x,
                                     isFixed: true, color: "#00B8D9"};
                        })}
                        spellCheck="false"/>
                    </InputGroup>
                  </Col>
                </Form.Group>

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={()=>{
                    this.saveSettings();
                    this.props.onClose();
                }}>
                  {T("Close")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    };
}

const UserSettingsDialogWithRouter = withRouter(UserSettingsDialog);

export default class UserLabel extends React.Component {
    static contextType = UserConfig;
    static propTypes = {
        setClient: PropTypes.func.isRequired,
    };

    state = {
        showUserSettings: false,
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    setSettings = (params) => {
        if (params.theme) {
            // Set the ACE theme according to the theme so they match.
            let ace_options = JSONparse(this.context.traits.ui_settings, {});
            if (params.theme === "no-theme") {
                ace_options.theme = "ace/theme/xcode";
                ace_options.fontFamily = "Iosevka Term";
            } else if (params.theme === "veloci-dark") {
                ace_options.theme = "ace/theme/vibrant_ink";
                ace_options.fontFamily = "Iosevka Term";
            } else if(params.theme === "veloci-light" || params.theme === "veloci-docs") {
                ace_options.theme = "ace/theme/xcode";
                ace_options.fontFamily = "Iosevka Term";
            } else if(params.theme === "pink-light") {
                ace_options.theme = "ace/theme/xcode";
                ace_options.fontFamily = "Iosevka Term";
            } else if(params.theme === "ncurses") {
                ace_options.theme = "ace/theme/sqlserver";
                ace_options.fontFamily = "fixedsys";
            } else if(params.theme === "ncurses-dark") {
                ace_options.theme = "ace/theme/tomorrow_night_eighties";
                ace_options.fontFamily = "fixedsys";
            } else if(params.theme === "github-default-light") {
                ace_options.theme = "ace/theme/dracula";
                ace_options.fontFamily = "Iosevka Term";
            } else if(params.theme === "github-dimmed-dark") {
                ace_options.theme = "ace/theme/dracula";
                ace_options.fontFamily = "Iosevka Term";
            } else if(params.theme === "coolgray-dark") {
                ace_options.theme = "ace/theme/nord_dark";
                ace_options.fontFamily = "Iosevka Term";
            } else if(params.theme === "midnight") {
                ace_options.theme = "ace/theme/terminal";
                ace_options.fontFamily = "Iosevka Term";
            } else if(params.theme === "vscode-dark") {
                ace_options.theme = "ace/theme/tomorrow_night";
                ace_options.fontFamily = "Iosevka Term";
            }

            params.options = JSON.stringify(ace_options);
        }

        if(_.isEmpty(params)) {
            return;
        }

        api.post("v1/SetGUIOptions", params,
                 this.source.token).then((response) => {
                     if (response.status === 200) {
                         // Check for redirect from the server - this
                         // is normally set by the authenticator to
                         // redirect to a better server.
                         if (response.data && response.data.redirect_url &&
                             response.data.redirect_url !== "") {
                             window.location.assign(response.data.redirect_url);
                         }
                     }

                     this.context.updateTraits();
                 }).catch(response=>{
                     // If we tried to set the org but we dont have
                     // permission in it, we switch rightr back to the
                     // previous org.
                     if (response.response &&
                         response.response.status === 403 &&
                         params.org) {
                         window.globals.OrgId = params.previous_org;
                         window.history.pushState(
                             {}, "", api.href("/app/index.html", {}));

                     }
                 });
    }

    orgName() {
        let id = window.globals.OrgId || (
            this.context.traits && this.context.traits.org);
        if (!id || id==="root") {
            return <></>;
        }
        let orgs = (this.context.traits && this.context.traits.orgs) || [];
        for(let i=0; i<orgs.length;i++) {
            if (orgs[i].id===id) {
                return <div className="org-label">{orgs[i].name}</div>;
            }
        };
        return <></>;
    }

    render() {
        return (
            <>
              { this.state.showUserSettings &&
                <UserSettingsDialogWithRouter
                  setClient={this.props.setClient}
                  setSetting={this.setSettings}
                  onClose={()=>this.setState({showUserSettings: false})} />
              }
              <ButtonGroup className="user-label">
                <Button href={api.href("/app/logoff.html", {
                    username: this.context.traits.username,
                })} >
                  <FontAwesomeIcon icon="sign-out-alt" />
                </Button>
                <Button variant="default"
                        onClick={()=>this.setState({showUserSettings: true})}
                >
                  { this.context.traits.username }
                  { this.context.traits.picture &&
                    <img className="toolbar-buttons img-thumbnail"
                         alt="avatar"
                         src={this.context.traits.picture}
                    />
                  }
                  { this.orgName() }
                </Button>
                <Button variant="default"
                  onClick={()=>this.setState({showUserSettings: true})}
                >
                  <FontAwesomeIcon icon="wrench" />
                </Button>
              </ButtonGroup>
            </>
        );
    }
};
