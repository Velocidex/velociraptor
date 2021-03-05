import './user-label.css';
import _ from 'lodash';

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

class UserSettings extends React.PureComponent {
    static contextType = UserConfig;
    static propTypes = {
        onClose: PropTypes.func.isRequired,
    }

    render() {
        let theme = (this.context.traits && this.context.traits.theme) || "light-theme";

        return (
            <Modal show={true} onHide={() => this.props.onClose()}>
              <Modal.Header closeButton>
                <Modal.Title>User Settings</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    Theme
                  </Form.Label>
                  <Col sm="8">
                    <Form.Control as="select"
                                  value={theme}
                                  placeholder="Select a theme"
                                  onChange={(e) => {
                                      this.props.onClose(e.currentTarget.value);
                                  }}>
                      <option value="light-mode">Light Mode</option>
                      <option value="dark-mode">Dark Mode</option>
                    </Form.Control>
                  </Col>
                </Form.Group>

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={() => this.props.onClose()}>
                  Cancel
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

    setSettings = (theme) => {
        this.setState({showUserSettings: false});
        if (_.isEmpty(theme)) {
            return;
        }
        let current_theme = this.context.traits && this.context.traits.theme;
        if (current_theme !== theme) {
            // Set the ACE theme according to the theme so they match.
            let ace_options = JSON.parse(this.context.traits.ui_settings || "{}");
            if (theme === "dark-mode") {
                ace_options.theme = "ace/theme/terminal";
            } else if(theme === "light-mode") {
                ace_options.theme = "ace/theme/xcode";
            }

            api.post("v1/SetGUIOptions", {
                options: JSON.stringify(ace_options),
                'theme': theme,
            }).then((response) => {
                this.context.updateTraits();
            });
        }
    }

    render() {
        return (
            <>
              { this.state.showUserSettings &&
                <UserSettings onClose={this.setSettings} /> }
              <ButtonGroup className="user-label">
                <Button href={api.base_path + "/logoff?username="+ this.context.traits.username } >
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
