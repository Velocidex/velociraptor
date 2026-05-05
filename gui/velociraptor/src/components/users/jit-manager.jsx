import React, { Component } from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Table from 'react-bootstrap/Table';
import Button from 'react-bootstrap/Button';
import Badge from 'react-bootstrap/Badge';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Container from 'react-bootstrap/Container';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import T from '../i8n/i8n.jsx';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import UserConfig from '../core/user.jsx';
import ToolTip from '../widgets/tooltip.jsx';

const POLL_TIME = 5000;

function formatTime(epoch_sec) {
    if (!epoch_sec) return "-";
    return new Date(epoch_sec * 1000).toLocaleString();
}

function formatTimeRemaining(expires_time) {
    if (!expires_time) return "-";
    let now = Date.now() / 1000;
    let remaining = expires_time - now;
    if (remaining <= 0) return T("Expired");
    let hours = Math.floor(remaining / 3600);
    let minutes = Math.floor((remaining % 3600) / 60);
    if (hours > 0) return hours + "h " + minutes + "m";
    return minutes + "m";
}

function statusBadge(status) {
    let variant = "secondary";
    let label = status;
    switch(status) {
    case 0: variant = "warning"; label = "PENDING"; break;
    case 1: variant = "success"; label = "APPROVED"; break;
    case 2: variant = "danger"; label = "DENIED"; break;
    case 3: variant = "secondary"; label = "EXPIRED"; break;
    case 4: variant = "dark"; label = "REVOKED"; break;
    }
    return <Badge bg={variant}>{T(label)}</Badge>;
}

class ApproveDialog extends Component {
    static propTypes = {
        request: PropTypes.object.isRequired,
        onClose: PropTypes.func.isRequired,
        onSubmit: PropTypes.func.isRequired,
        approve: PropTypes.bool.isRequired,
    }

    state = {
        reason: "",
    }

    render() {
        let title = this.props.approve ?
            T("Approve JIT Request") : T("Deny JIT Request");
        let variant = this.props.approve ? "success" : "danger";
        let req = this.props.request;

        return (
            <Modal show={true} onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{title}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <p><strong>{T("Requester")}:</strong> {req.requester}</p>
                <p><strong>{T("Roles")}:</strong> {(req.roles || []).join(", ")}</p>
                <p><strong>{T("Justification")}:</strong> {req.justification}</p>
                <p><strong>{T("Duration")}:</strong> {Math.round(req.duration_sec / 60)} {T("minutes")}</p>
                <Form.Group className="mb-3">
                  <Form.Label>{T("Reason (optional)")}</Form.Label>
                  <Form.Control
                    as="textarea"
                    rows={2}
                    value={this.state.reason}
                    onChange={e => this.setState({reason: e.target.value})}
                  />
                </Form.Group>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  {T("Cancel")}
                </Button>
                <Button variant={variant}
                  onClick={() => this.props.onSubmit(this.state.reason)}>
                  {this.props.approve ? T("Approve") : T("Deny")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


class JITApprovalManager extends Component {
    static contextType = UserConfig;

    state = {
        requests: [],
        showApproveDialog: false,
        showDenyDialog: false,
        selectedRequest: null,
    }

    componentDidMount() {
        this.source = CancelToken.source();
        this.interval = setInterval(this.loadRequests, POLL_TIME);
        this.loadRequests();
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }

    loadRequests = () => {
        this.source.cancel();
        this.source = CancelToken.source();

        api.get("v1/JITList", {},
                this.source.token).then(response => {
            if (response.cancel) return;
            this.setState({requests: response.data.items || []});
        });
    }

    approveOrDeny = (approve, reason) => {
        let req = this.state.selectedRequest;
        if (!req) return;

        api.post("v1/JITApprove", {
            request_id: req.request_id,
            approve: approve,
            reason: reason,
        }, this.source.token).then(response => {
            if (response.cancel) return;
            this.setState({
                showApproveDialog: false,
                showDenyDialog: false,
                selectedRequest: null,
            });
            this.loadRequests();
        });
    }

    revokeGrant = (request_id) => {
        api.post("v1/JITRevoke", {
            request_id: request_id,
        }, this.source.token).then(response => {
            if (response.cancel) return;
            this.loadRequests();
        });
    }

    render() {
        let pending = _.filter(this.state.requests, r => r.status === 0);
        let others = _.filter(this.state.requests, r => r.status !== 0);

        return (
            <Container className="jit-manager">
              {this.state.showApproveDialog && this.state.selectedRequest &&
               <ApproveDialog
                 request={this.state.selectedRequest}
                 approve={true}
                 onClose={() => this.setState({showApproveDialog: false})}
                 onSubmit={(reason) => this.approveOrDeny(true, reason)}
               />}
              {this.state.showDenyDialog && this.state.selectedRequest &&
               <ApproveDialog
                 request={this.state.selectedRequest}
                 approve={false}
                 onClose={() => this.setState({showDenyDialog: false})}
                 onSubmit={(reason) => this.approveOrDeny(false, reason)}
               />}

              <h5>{T("Pending Requests")}</h5>
              {_.isEmpty(pending) ?
               <p className="text-muted">{T("No pending JIT requests")}</p> :
               <Table striped bordered size="sm">
                 <thead>
                   <tr>
                     <th>{T("Requester")}</th>
                     <th>{T("Roles")}</th>
                     <th>{T("Justification")}</th>
                     <th>{T("Duration")}</th>
                     <th>{T("Requested")}</th>
                     <th>{T("Actions")}</th>
                   </tr>
                 </thead>
                 <tbody>
                   {_.map(pending, (req, idx) => (
                       <tr key={idx}>
                         <td>{req.requester}</td>
                         <td>{(req.roles || []).map(r =>
                             <Badge key={r} bg="info" className="me-1">
                               {T("Role_" + r)}
                             </Badge>)}
                         </td>
                         <td>{req.justification}</td>
                         <td>{Math.round(req.duration_sec / 60)}m</td>
                         <td>{formatTime(req.created_time)}</td>
                         <td>
                           <ToolTip tooltip={T("Approve")}>
                             <Button size="sm" variant="success"
                               className="me-1"
                               onClick={() => this.setState({
                                   showApproveDialog: true,
                                   selectedRequest: req,
                               })}>
                               <FontAwesomeIcon icon="check"/>
                             </Button>
                           </ToolTip>
                           <ToolTip tooltip={T("Deny")}>
                             <Button size="sm" variant="danger"
                               onClick={() => this.setState({
                                   showDenyDialog: true,
                                   selectedRequest: req,
                               })}>
                               <FontAwesomeIcon icon="times"/>
                             </Button>
                           </ToolTip>
                         </td>
                       </tr>
                   ))}
                 </tbody>
               </Table>
              }

              <h5 className="mt-4">{T("All Requests")}</h5>
              {_.isEmpty(others) ?
               <p className="text-muted">{T("No JIT request history")}</p> :
               <Table striped bordered size="sm">
                 <thead>
                   <tr>
                     <th>{T("Requester")}</th>
                     <th>{T("Roles")}</th>
                     <th>{T("Status")}</th>
                     <th>{T("Approver")}</th>
                     <th>{T("Expires")}</th>
                     <th>{T("Actions")}</th>
                   </tr>
                 </thead>
                 <tbody>
                   {_.map(others, (req, idx) => (
                       <tr key={idx}>
                         <td>{req.requester}</td>
                         <td>{(req.roles || []).map(r =>
                             <Badge key={r} bg="info" className="me-1">
                               {T("Role_" + r)}
                             </Badge>)}
                         </td>
                         <td>{statusBadge(req.status)}</td>
                         <td>{req.approver || "-"}</td>
                         <td>{req.status === 1 ?
                              formatTimeRemaining(req.expires_time) : "-"}
                         </td>
                         <td>
                           {req.status === 1 &&
                            <ToolTip tooltip={T("Revoke")}>
                              <Button size="sm" variant="warning"
                                onClick={() => this.revokeGrant(req.request_id)}>
                                <FontAwesomeIcon icon="ban"/>
                              </Button>
                            </ToolTip>}
                         </td>
                       </tr>
                   ))}
                 </tbody>
               </Table>
              }
            </Container>
        );
    }
}


class JITMyGrants extends Component {
    state = {
        grants: [],
    }

    componentDidMount() {
        this.source = CancelToken.source();
        this.interval = setInterval(this.loadGrants, POLL_TIME);
        this.loadGrants();
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }

    loadGrants = () => {
        this.source.cancel();
        this.source = CancelToken.source();

        api.get("v1/JITMyGrants", {},
                this.source.token).then(response => {
            if (response.cancel) return;
            this.setState({grants: response.data.items || []});
        });
    }

    render() {
        if (_.isEmpty(this.state.grants)) {
            return <p className="text-muted p-3">
                     {T("No active JIT grants")}
                   </p>;
        }

        return (
            <Table striped bordered size="sm">
              <thead>
                <tr>
                  <th>{T("Roles")}</th>
                  <th>{T("Approved By")}</th>
                  <th>{T("Time Remaining")}</th>
                </tr>
              </thead>
              <tbody>
                {_.map(this.state.grants, (grant, idx) => (
                    <tr key={idx}>
                      <td>{(grant.roles || []).map(r =>
                          <Badge key={r} bg="info" className="me-1">
                            {T("Role_" + r)}
                          </Badge>)}
                      </td>
                      <td>{grant.approver}</td>
                      <td>{formatTimeRemaining(grant.expires_time)}</td>
                    </tr>
                ))}
              </tbody>
            </Table>
        );
    }
}

export { JITApprovalManager, JITMyGrants };
export default JITApprovalManager;
