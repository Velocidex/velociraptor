import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import Form from 'react-bootstrap/Form';
import _ from 'lodash';
import T from '../i8n/i8n.jsx';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import UserConfig from '../core/user.jsx';

// org_admin are blocked server-side due to elevated privileges
const ALL_ROLES = ["analyst", "investigator", "artifact_writer", "administrator"];

function getAvailableRoles(permissions) {
    if (!permissions) return ALL_ROLES;

    if (permissions.server_admin) return [];

    let hidden = [];

    // Investigator permissions subsume analyst
    if (permissions.collect_client && permissions.start_hunt) {
        hidden.push("investigator", "analyst");
    } else if (permissions.notebook_editor && permissions.any_query) {
        hidden.push("analyst");
    }

    if (permissions.artifact_writer) hidden.push("artifact_writer");

    return _.without(ALL_ROLES, ...hidden);
}

const DURATION_OPTIONS = [
    { value: 1800, label: "30 minutes" },
    { value: 3600, label: "1 hour" },
    { value: 7200, label: "2 hours" },
    { value: 14400, label: "4 hours" },
];

class JITRequestDialog extends Component {
    static contextType = UserConfig;
    static propTypes = {
        onClose: PropTypes.func.isRequired,
        onSubmit: PropTypes.func,
    }

    state = {
        selectedRoles: [],
        justification: "",
        duration_sec: 3600,
        error: "",
        submitting: false,
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    toggleRole = (role) => {
        let roles = [...this.state.selectedRoles];
        if (_.includes(roles, role)) {
            roles = _.without(roles, role);
        } else {
            roles.push(role);
        }
        this.setState({selectedRoles: roles});
    }

    submit = () => {
        if (_.isEmpty(this.state.selectedRoles)) {
            this.setState({error: T("Please select at least one role")});
            return;
        }
        if (!this.state.justification.trim()) {
            this.setState({error: T("Justification is required")});
            return;
        }

        this.setState({submitting: true, error: ""});

        api.post("v1/JITRequestRole", {
            roles: this.state.selectedRoles,
            justification: this.state.justification,
            duration_sec: this.state.duration_sec,
        }, this.source.token).then(response => {
            if (response.cancel) return;
            this.setState({submitting: false});
            if (this.props.onSubmit) {
                this.props.onSubmit(response.data);
            }
            this.props.onClose();
        }).catch(err => {
            this.setState({
                submitting: false,
                error: err.response?.data || "Request failed",
            });
        });
    }

    render() {
        let permissions = this.context.traits && this.context.traits.Permissions;
        let availableRoles = getAvailableRoles(permissions);

        return (
            <Modal show={true} onHide={this.props.onClose} size="lg">
              <Modal.Header closeButton>
                <Modal.Title>{T("Request Temporary Role")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {this.state.error &&
                 <div className="alert alert-danger">{this.state.error}</div>}

                {_.isEmpty(availableRoles) ?
                 <div className="alert alert-info">
                   {T("You already have all available roles")}
                 </div> :
                 <Form.Group className="mb-3">
                  <Form.Label>{T("Select Roles")}</Form.Label>
                  {_.map(availableRoles, (role) => (
                      <Form.Check
                        key={role}
                        type="checkbox"
                        id={"jit-role-" + role}
                        label={T("Role_" + role)}
                        checked={_.includes(this.state.selectedRoles, role)}
                        onChange={() => this.toggleRole(role)}
                      />
                  ))}
                </Form.Group>
                }

                <Form.Group className="mb-3">
                  <Form.Label>{T("Duration")}</Form.Label>
                  <Form.Select
                    value={this.state.duration_sec}
                    onChange={e => this.setState({
                        duration_sec: parseInt(e.target.value)
                    })}>
                    {_.map(DURATION_OPTIONS, (opt) => (
                        <option key={opt.value} value={opt.value}>
                          {opt.label}
                        </option>
                    ))}
                  </Form.Select>
                </Form.Group>

                <Form.Group className="mb-3">
                  <Form.Label>{T("Justification")}</Form.Label>
                  <Form.Control
                    as="textarea"
                    rows={3}
                    placeholder={T("Explain why you need this role...")}
                    value={this.state.justification}
                    onChange={e => this.setState({
                        justification: e.target.value
                    })}
                  />
                </Form.Group>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  {T("Cancel")}
                </Button>
                <Button
                  variant="primary"
                  disabled={this.state.submitting || _.isEmpty(availableRoles)}
                  onClick={this.submit}>
                  {this.state.submitting ? T("Submitting...") : T("Submit Request")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

export default JITRequestDialog;
