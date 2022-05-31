import './user-label.css';
import _ from 'lodash';
import moment from 'moment';

import React from 'react';
import PropTypes from 'prop-types';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

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
                default_password: this.context.traits.default_password || "",
            });
        }
    }

    state = {
        theme: "",
        timezone: "",
        default_password: "",
    }

    saveSettings = ()=> {
        this.props.setSetting({
            theme: this.state.theme,
            timezone: this.state.timezone,
            lang: this.state.lang,
            default_password: this.state.default_password,
        });
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
                      {/* <option value="ncurses">Ncurses (light)</option> */}
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
        ace_options.fontSize = "18px";
        if (options.theme === "no-theme") {
            ace_options.theme = "ace/theme/xcode";
        } else if (options.theme === "veloci-dark") {
            ace_options.theme = "ace/theme/terminal";
        } else if(options.theme === "veloci-light") {
            ace_options.theme = "ace/theme/xcode";
        } else if(options.theme === "pink-light") {
            ace_options.theme = "ace/theme/xcode";
        } else if(options.theme === "ncurses") {
            ace_options.theme = "ace/theme/github";
        } else if(options.theme === "github-dimmed-dark") {
          ace_options.theme = "ace/theme/vibrant_ink";
        } else if(options.theme === "github-dimmed-light") {
          ace_options.theme = "ace/theme/sqlserver";
        } else if(options.theme === "coolgray-dark") {
          ace_options.theme = "ace/theme/nord_dark";
        }
        options.options = JSON.stringify(ace_options);

        api.post("v1/SetGUIOptions", options, this.source.token).then((response) => {
            this.context.updateTraits();
        });
    }

    render() {
        return (
            <>
              { this.state.showUserSettings &&
                <UserSettings
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
                </Button>
              </ButtonGroup>
            </>
        );
    }
};
