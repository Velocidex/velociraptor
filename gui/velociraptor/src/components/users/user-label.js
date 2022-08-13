import './user-label.css';
import _ from 'lodash';
import moment from 'moment';

import React from 'react';
import PropTypes from 'prop-types';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import { withRouter }  from "react-router-dom";

import Accordion from 'react-bootstrap/Accordion';
import Card  from 'react-bootstrap/Card';
import UserConfig from '../core/user.js';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import api from '../core/api-service.js';
import axios from 'axios';
import VeloForm from '../forms/form.js';
import T from '../i8n/i8n.js';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import Tooltip from 'react-bootstrap/Tooltip';
import InputGroup from 'react-bootstrap/InputGroup';
import Select from 'react-select';


class _PasswordChange extends React.Component {
    static propTypes = {
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount() {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    updatePassword = ()=>{
        api.post("v1/SetPassword", {
            password: this.state.password1,
        }, this.source.token).then((response) => {
            this.props.history.push("/welcome");
            this.props.onClose();
        });
    }

    state = {
        password1: "",
        password2: "",
    }

    render() {
        return  (
            <Form.Group as={Row}>
              <Form.Label column sm="3">
                {T("Password")}
              </Form.Label>
              <Col sm="8">
                <Accordion>
                  <Card>
                    <Accordion.Toggle as={Card.Header} eventKey="0">
                      {T("Update Password")}
                      <span className="float-right">
                        <FontAwesomeIcon icon="chevron-down"/>
                      </span>
                    </Accordion.Toggle>
                    <Accordion.Collapse eventKey="0">
                      <>
                        <Form.Control type="password"
                                      value={this.state.password1}
                                      onChange={e=>this.setState({password1: e.currentTarget.value})}
                                      placeholder={T("Password")} />
                        <Form.Control type="password"
                                      value={this.state.password2}
                                      onChange={e=>this.setState({password2: e.currentTarget.value})}
                                      placeholder={T("Retype Password")} />
                        { this.state.password1 && this.state.password2 &&
                          <Button variant={this.state.password1 !== this.state.password2 ? "warning" : "default"}
                                  size="sm"
                                  className="set-password-button"
                                  disabled={this.state.password1 !== this.state.password2}
                                  onClick={this.updatePassword}
                          >
                            {this.state.password1 !== this.state.password2 ? T("Passwords do not match") : T("Submit")}
                          </Button>
                        }
                      </>
                    </Accordion.Collapse>
                  </Card>
                </Accordion>
              </Col>
            </Form.Group>
        );
    }
}

const PasswordChange = withRouter(_PasswordChange);


class UserSettings extends React.PureComponent {
    static contextType = UserConfig;
    static propTypes = {
        onClose: PropTypes.func.isRequired,
        setSetting: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        if (this.context.traits) {
            this.setState({
                theme: this.context.traits.theme || "no-theme",
                timezone: this.context.traits.timezone || "UTC",
                lang: this.context.traits.lang || "en",
                org: this.context.traits.org || "root",
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
    }

    saveSettings = ()=> {
        this.props.setSetting({
            theme: this.state.theme,
            timezone: this.state.timezone,
            lang: this.state.lang,
            org: this.state.org,
            default_password: this.state.default_password,
        });
    }

    changeOrg = org=>{
        // Force navigation to the
        // welcome screen to make sure
        // the GUI is reset
        this.props.history.push("/welcome");
        this.props.setSetting({
            theme: this.state.theme,
            timezone: this.state.timezone,
            lang: this.state.lang,
            org: org,
            default_password: this.state.default_password,
        });
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
                      <OverlayTrigger
                        delay={{show: 250, hide: 400}}
                        overlay={(props)=><Tooltip {...props}>
                                  {T("Switch to a different org")}
                                </Tooltip>}>
                        <div>{T("Organization")}</div>
                      </OverlayTrigger>
                    </Form.Label>
                    <Col sm="8">
                      <Form.Control as="select"
                                    value={this.state.org}
                                    placeholder={T("Select an org")}
                                    onChange={e=>this.changeOrg(e.currentTarget.value)}
                  >
                        {_.map(this.context.traits.orgs || [], function(x) {
                            return <option key={x.id} value={x.id}>{x.name}</option>;
                        })}
                      </Form.Control>
                    </Col>
                  </Form.Group>
                }
                { !this.context.traits.password_less &&
                  <PasswordChange
                    onClose={this.props.onClose}
                    >
                  </PasswordChange> }
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
                      <option value="no-theme">{T("Default Velociraptor")}</option>
                      <option value="veloci-light">{T("Velociraptor (light)")}</option>
                      <option value="veloci-dark">{T("Velociraptor (dark)")}</option>
                      {/* <option value="github-dimmed-light">Github dimmed (light)</option> */}
                      <option value="github-dimmed-dark">{T("Github dimmed (dark)")}</option>
                      <option value="ncurses">{T("Ncurses (light)")}</option>
                      <option value="coolgray-dark">{T("Cool Gray (dark)")}</option>
                      <option value="pink-light">{T("Strawberry Milkshake (light)")}</option>
                    </Form.Control>
                  </Col>
                </Form.Group>
                <VeloForm
                  param={{name: "Downloads Password",
                          friendly_name: T("Downloads Password"),
                          description: T("Default password to use for downloads"),
                          type: "string"}}
                  value={this.state.default_password}
                  setValue={value=>this.setState({default_password: value})}
                />
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=><Tooltip {...props}>
                                          {T("Select a language")}
                                        </Tooltip>}>
                      <div>{T("Language")}</div>
                    </OverlayTrigger>
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
                                      this.props.setSetting({
                                          lang: e.currentTarget.value,
                                      });
                                  }}>
                      <option value="en">{T("English")}</option>
                      <option value="de">{T("Deutsch")}</option>
                      <option value="es">{T("Spanish")}</option>
                      <option value="por">{T("Portuguese")}</option>
                      <option value="fr">{T("French")}</option>
                      <option value="jp">{T("Japanese")}</option>
                    </Form.Control>
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=><Tooltip {...props}>
                                          {T("Select a timezone")}
                                        </Tooltip>}>
                      <div>{T("Display timezone")}</div>
                    </OverlayTrigger>
                  </Form.Label>
                  <Col sm="8">
                    <InputGroup className="mb-3">
                      <InputGroup.Prepend>
                        <InputGroup.Text
                          as="button"
                          className="btn btn-default"
                          onClick={()=>{
                              this.setState({timezone: "UTC"});
                              this.props.setSetting({
                                  timezone: "UTC",
                              });
                          }}>
                          UTC
                        </InputGroup.Text>
                      </InputGroup.Prepend>
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
                  {T("Save")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    };
}

const UserSettingsWithRouter = withRouter(UserSettings);

export default class UserLabel extends React.Component {
    static contextType = UserConfig;
    static propTypes = {

    };

    state = {
        showUserSettings: false,
    }

    componentDidMount() {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    setSettings = (options) => {
        // Set the ACE theme according to the theme so they match.
        let ace_options = JSON.parse(this.context.traits.ui_settings || "{}");
        if (options.theme === "no-theme") {
            ace_options.theme = "ace/theme/xcode";
            ace_options.fontFamily = "monospace";
        } else if (options.theme === "veloci-dark") {
            ace_options.theme = "ace/theme/vibrant_ink";
            ace_options.fontFamily = "monospace";
        } else if(options.theme === "veloci-light") {
            ace_options.theme = "ace/theme/xcode";
            ace_options.fontFamily = "monospace";
        } else if(options.theme === "pink-light") {
            ace_options.theme = "ace/theme/xcode";
            ace_options.fontFamily = "monospace";
        } else if(options.theme === "ncurses") {
            ace_options.theme = "ace/theme/sqlserver";
            ace_options.fontFamily = "fixedsys";
        } else if(options.theme === "github-dimmed-dark") {
          ace_options.theme = "ace/theme/vibrant_ink";
          ace_options.fontFamily = "monospace";
        } else if(options.theme === "github-dimmed-light") {
          ace_options.theme = "ace/theme/sqlserver";
          ace_options.fontFamily = "monospace";
        } else if(options.theme === "coolgray-dark") {
          ace_options.theme = "ace/theme/nord_dark";
          ace_options.fontFamily = "monospace";
        }
        options.options = JSON.stringify(ace_options);

        api.post("v1/SetGUIOptions", options, this.source.token).then((response) => {
            this.context.updateTraits();
        });
    }

    orgName() {
        let id = this.context.traits && this.context.traits.org;
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
                <UserSettingsWithRouter
                  setSetting={this.setSettings}
                  onClose={()=>this.setState({showUserSettings: false})} /> }
              <ButtonGroup className="user-label">
                <Button href={api.base_path + "/app/logoff.html?username="+
                              this.context.traits.username } >
                  <FontAwesomeIcon icon="sign-out-alt" />
                </Button>
                <Button variant="default"
                        onClick={()=>this.setState({showUserSettings: true})}
                >
                  { this.context.traits.username }
                  { this.context.traits.picture &&
                    <img className="toolbar-buttons img-thumbnail"
                         alt="avatar"
                         src={ this.context.traits.picture}
                    />
                  }
                  { this.orgName() }
                </Button>
              </ButtonGroup>
            </>
        );
    }
};
