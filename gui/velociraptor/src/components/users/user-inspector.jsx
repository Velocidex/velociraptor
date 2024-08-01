import _ from 'lodash';

import "./user.css";

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Nav from 'react-bootstrap/Nav';
import T from '../i8n/i8n.jsx';
import { withRouter }  from "react-router-dom";
import UserConfig from '../core/user.jsx';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Container from  'react-bootstrap/Container';
import Table  from 'react-bootstrap/Table';
import Card  from 'react-bootstrap/Card';
import Button from 'react-bootstrap/Button';
import Form from 'react-bootstrap/Form';
import Modal from 'react-bootstrap/Modal';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import AddOrgDialog from './add_orgs.jsx';
import AddUserDialog from './add_user.jsx';
import EditUserDialog from './edit-user.jsx';

import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import ToolTip from '../widgets/tooltip.jsx';

const POLL_TIME = 5000;

function getOrgRecordsForUser(users, name) {
    if (_.isEmpty(users)) {
        return [];
    }

    for (let i=0; i<users.length; i++) {
        if (users[i].name === name) {
            return _.map(users[i].orgs, x=>x);
        }
    }
    return [];
}


class ConfirmDialog extends Component {
    static propTypes = {
        onClose: PropTypes.func.isRequired,
        onSubmit: PropTypes.func.isRequired,
        org_name: PropTypes.string,
        user_name: PropTypes.string,
    }

    render() {
        return(
            <Modal show={true}
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Warning")}</Modal.Title>
              </Modal.Header>
              <Modal.Body >
                {T("WARN_REMOVE_USER_FROM_ORG", this.props.user_name,
                   this.props.org_name)}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        onClick={this.props.onSubmit}>
                  {T("Do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

class PermissionViewer extends Component {
    static propTypes = {
        username: PropTypes.string,
        org: PropTypes.object,
        acls: PropTypes.object,
        setACL: PropTypes.func.isRequired,
    }

    state = {
        showHelpDialog: false,
        showConfirmDeleteDialog: false,
        message: "",
        help_topic: "",
        pending_acl: {}
    }

    changeRole = (role, role_is_set) => {
        let new_acl = {...this.props.acls};
        let roles = new_acl.roles || [];
        if (role_is_set && _.indexOf(roles, role) < 0) {
            roles.push(role);
            new_acl.roles  = roles;
            this.props.setACL(new_acl);
        }
        if (!role_is_set) {
            roles = _.filter(roles, x=>x !== role);
            new_acl.roles  = roles;

            // We are about to remove all permissions from the
            // user. Stop and wait for the user to confirm this is
            // what they want because it will delete the user from the
            // org.
            if (_.isEmpty(new_acl.roles) &&
                _.isEmpty(new_acl.permissions)) {
                this.setState({pending_acl: new_acl,
                               showConfirmDeleteDialog: true});
                return;
            }
            this.props.setACL(new_acl);
        }
    }

    changePermissions = (perm, perm_is_set) => {
        let new_acl = {...this.props.acls};
        let perms = new_acl.permissions || [];
        if (perm_is_set && _.indexOf(perms, perm) < 0) {
            perms.push(perm);
            new_acl.permissions = perms;
            this.props.setACL(new_acl);
        }
        if (!perm_is_set) {
            perms = _.filter(perms, x=>x !== perm);
            new_acl.permissions  = perms;
            this.props.setACL(new_acl);
        }
    }

    render() {
        if (!this.props.username) {
            return <></>;
        }

        if (_.isEmpty(this.props.org)) {
            return <div className="no-content">
                     <FontAwesomeIcon icon="angles-left"
                                      className="fa-fade"/>
                     {T("Please Select an Org")}
                   </div>;
        }

        if (_.isEmpty(this.props.acls)) {
            return <div className="no-content">
                       {T("Loading ACLs")}
                       </div> ;
        }

        let org_name = this.props.acls.org_name || "root";
        return (
            <Container className="permission-viewer">
              <Modal show={this.state.showHelpDialog}
                     onHide={() => this.setState({showHelpDialog: false})}>
                <Modal.Header closeButton>
                  <Modal.Title>{T(this.state.help_topic)}</Modal.Title>
                </Modal.Header>
                <Modal.Body >
                  {T(this.state.message)}
                </Modal.Body>
              </Modal>

              {this.state.showConfirmDeleteDialog &&
               <ConfirmDialog
                 org_name={org_name}
                 user_name={this.props.acls.name}
                 onClose={()=>this.setState({showConfirmDeleteDialog: false})}
                 onSubmit={()=>{
                     this.props.setACL(this.state.pending_acl);
                     this.setState({showConfirmDeleteDialog: false});
                 }}
               /> }

              <Card>
                <Card.Header>
                  {T("Roles")} - {this.props.acls.name} @ {org_name}
                </Card.Header>
                <Card.Body>
                  { _.map(this.props.acls.all_roles, (role, idx)=>(
                        <div className="form-toggle" key={role}>
                          <Button
                            variant="outline-default"
                            as="button"
                            onClick={()=>this.setState({
                                showHelpDialog: true,
                                help_topic: "Role_" + role,
                                message: "ToolRole_" + role})}>
                            <FontAwesomeIcon icon="info"/>
                          </Button>

                          <div className="form-toggle-control">
                            <Form.Switch
                              id={"Role_" + role}
                              label={T("Role_" + role)}
                              checked={_.indexOf(this.props.acls.roles, role) >= 0}
                              onChange={e=>this.changeRole(
                                  role, e.currentTarget.checked)} />
                          </div>
                        </div>
                      ))}
                </Card.Body>
              </Card>

              { this.props.acls.permissions &&
                <Card>
                  <Card.Header>
                    {T("Extra Permissions")}
                  </Card.Header>
                  <Card.Body>
                    { _.map(this.props.acls.permissions, (perm, idx)=>(
                        <div className="form-toggle" key={perm}>
                          <Button
                            variant="outline-default"
                            as="button"
                            onClick={()=>this.setState({
                                showHelpDialog: true,
                                help_topic: "Perm_" + perm,
                                message: "ToolPerm_" + perm})}>
                            <FontAwesomeIcon icon="info"/>
                          </Button>

                          <div className="form-toggle-control">
                            <Form.Switch
                              id={"Perm_" + perm}
                              label={T("Perm_" + perm)}
                              checked={true}
                              onChange={e=>this.changePermissions(
                                  perm, e.currentTarget.checked)} />
                          </div>
                        </div>
                    ))}
                  </Card.Body>
                </Card>
              }

              <Card>
                <Card.Header>
                  {T("Effective Permissions")}
                </Card.Header>
                <Card.Body>
                  { _.map(this.props.acls.all_permissions, (perm, idx)=>(
                      <div className="form-toggle" key={perm}>
                        <Button
                          variant="outline-default"
                          as="button"
                          onClick={()=>this.setState({
                              showHelpDialog: true,
                              help_topic: "Perm_" + perm,
                              message: "ToolPerm_" + perm})}>
                          <FontAwesomeIcon icon="info"/>
                        </Button>
                        <div className="form-toggle-control">
                          <Form.Switch
                            id={"Perm2_" + perm}
                            label={T("Perm_" + perm)}
                            disabled={_.indexOf(
                                this.props.acls.effective_permissions, perm) >= 0}
                            checked={_.indexOf(
                                this.props.acls.effective_permissions, perm) >= 0}
                            onChange={e=>this.changePermissions(
                                perm, e.currentTarget.checked)} />
                        </div>
                      </div>
                  ))}
                </Card.Body>
              </Card>

            </Container>
        );
    }
}


class UsersOverview extends Component {
    static contextType = UserConfig;

    static propTypes = {
        users: PropTypes.array,
        updateUsers: PropTypes.func.isRequired,
    }

    state = {
        user_name: "",
        org: {},
        acl: {},

        showAddUserDialog: false,
        showAddOrgDialog: false,
        showEditUserDialog: false,
    }

    componentDidMount = () => {
        this.setACLsource = CancelToken.source();
        this.getACLsource = CancelToken.source();
    }

    componentWillUnmount() {
        this.setACLsource.cancel();
        this.getACLsource.cancel();
    }

    setACL = (acl) => {
        this.setACLsource.cancel();
        this.setACLsource = CancelToken.source();

        api.post("v1/SetUserRoles", acl,
                 this.setACLsource.token).then(response=>{
                     if (response.cancel)
                         return;
                     this.getACL(acl.name,
                                 {id: acl.org, name: acl.org_name});
                     this.props.updateUsers();
                 });
    }

    getACL = (user_name, org) => {
        this.getACLsource.cancel();
        this.getACLsource = CancelToken.source();

        this.setState({acl: {}});
        let org_id = org && org.id;

        if (!user_name || !org_id) {
            return;
        }

        api.get("v1/GetUserRoles", {
            name: user_name,
            org: org_id,
        }, this.getACLsource.token).then(response=>{
            if (response.cancel)
                return;
            this.setState({acl: response.data});
        });
    }

    render() {
        let selected_orgs = getOrgRecordsForUser(
            this.props.users, this.state.user_name);

        return (
            <Row>
              { this.state.showAddOrgDialog &&
                <AddOrgDialog
                  user_name={this.state.user_name}
                  users={this.props.users}
                  onClose={()=>{
                      this.setState({showAddOrgDialog: false});
                      this.props.updateUsers();
                  }}
                /> }

              { this.state.showAddUserDialog &&
                <AddUserDialog
                  org={this.state.org.id || "root" }
                  onClose={()=>{
                      this.setState({showAddUserDialog: false});
                  }}
                  onSubmit={()=>{
                      this.setState({showAddUserDialog: false});
                      this.props.updateUsers();
                  }}
                /> }

              { this.state.showEditUserDialog &&
                <EditUserDialog
                  username={this.state.user_name }
                  onClose={()=>{
                      this.setState({showEditUserDialog: false});
                  }}
                  onSubmit={()=>{
                      this.setState({showEditUserDialog: false});
                      this.props.updateUsers();
                  }}
                /> }


              <Col sm="4">
                  <Container className="selectable user-list">
                    <Table size="sm">
                      <thead>
                        <tr>
                          <th>
                            {T("Users")}
                            { !this.context.traits.password_less &&
                              <ToolTip tooltip={T("Update User Password")}>
                                <Button
                                  disabled={!this.state.user_name}
                                  onClick={()=>this.setState({
                                      showEditUserDialog: true
                                  })}
                                  className="new-user-btn"
                                  variant="outline-default"
                                  as="button">
                                  <FontAwesomeIcon icon="edit"/>
                                  <span className="sr-only">
                                    {T("Update User Password")}
                                  </span>
                                </Button>
                              </ToolTip>
                            }
                            <ToolTip tooltip={T("Add a new user")}>
                              <Button
                                onClick={()=>this.setState({
                                    showAddUserDialog: true
                                })}
                                className="new-user-btn"
                                variant="outline-default"
                                as="button">
                                <FontAwesomeIcon icon="plus"/>
                                <span className="sr-only">
                                  {T("Add a new user")}
                                </span>
                              </Button>
                            </ToolTip>
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        { _.map(this.props.users, (item, idx)=>{
                            return <tr key={idx} className={
                                    this.state.user_name === item.name ?
                                    "row-selected" : undefined
                                }>
                                     <td onClick={e=>{
                                         this.setState({user_name: item.name});
                                         this.getACL(item.name, this.state.org);
                                     }}>
                                       {item.name}
                                     </td>
                                   </tr>;
                        })}
                      </tbody>
                    </Table>
                  </Container>
              </Col>
              <Col sm="4">
                  <Container className="selectable org-list">
                    <Table size="sm">
                      <thead>
                        <tr>
                          <th>
                            {T("Orgs")}
                            <ToolTip tooltip={T("Assign user to Orgs")}>
                              <Button
                                disabled={!this.state.user_name}
                                onClick={()=>this.setState({
                                    showAddOrgDialog: true
                                })}
                                className="new-user-btn"
                                variant="outline-default"
                                as="button">
                                <FontAwesomeIcon icon="plus"/>
                                <span className="sr-only">
                                  {T("Assign user to Orgs")}
                                </span>
                              </Button>
                            </ToolTip>
                          </th></tr>
                      </thead>
                      <tbody>
                      { _.isEmpty(selected_orgs) &&
                        <tr className="no-content">
                          <td>
                              <FontAwesomeIcon icon="angles-left"
                                               className="fa-fade"/>
                              {T("Please Select a User")}
                            </td>
                          </tr> }
                        { _.map(selected_orgs, (item, idx)=>{
                            return <tr key={idx} className={
                                this.state.org &&
                                    this.state.org.name === item.name ?
                                    "row-selected" : undefined}>
                                     <td onClick={e=>{
                                         this.setState({org: item});
                                         this.getACL(this.state.user_name, item);
                                     }}>
                                       {item.name}
                                     </td>
                                   </tr>;
                        })}
                      </tbody>
                    </Table>
                  </Container>
              </Col>
              <Col sm="4">
                <PermissionViewer
                  username={this.state.user_name}
                  org={this.state.org}
                  acls={this.state.acl}
                  setACL={this.setACL}
                />
              </Col>
            </Row>
        );
    }
}

class OrgsOverview extends UsersOverview {

    render() {
        // Invert the users data structure to show all the users
        // indexed by org.
        let orgs = {};
        let org_names_by_id = {};
        let org_id_by_name = {};
        _.each(this.props.users, user=>{
            _.each(user.orgs, org=>{
                let record = orgs[org.id] || [];
                record.push(user);
                orgs[org.id] = record;
                org_names_by_id[org.id] = org.name;
                org_id_by_name[org.name]=org.id;
            });
        });

        let org_names = _.keys(org_id_by_name).sort();
        let selected_org = this.state.org && this.state.org.id;
        let selected_users = (orgs[selected_org] || []).sort();

        return (
            <Row>
                 <Col sm="4">
                   <Container className="selectable org-list">
                    <Table size="sm">
                     <thead>
                       <tr><th>{T("Orgs")}</th></tr>
                       </thead>
                     <tbody>
                        { _.map(org_names, (name, idx)=>{
                            return <tr key={idx} className={
                                this.state.org &&
                                    this.state.org.name === name ?
                                    "row-selected" : undefined
                            }>
                                      <td onClick={e=>{
                                          this.setState({
                                              org: {
                                                  id: org_id_by_name[name],
                                                  name: name,
                                              }});
                                          this.getACL(
                                              this.state.user_name,
                                              {id: org_id_by_name[name]});
                                      }}>
                        {name}
                        </td>
                                      </tr>;
                        })}
                        </tbody>
                     </Table>
                    </Container>
                   </Col>
                 <Col sm="4">
            <Container className="selectable user-list">
             <Table size="sm">
                     <thead>
                       <tr><th>{T("Users")}</th></tr>
                     </thead>
                     <tbody>
                 { _.map(selected_users, (item, idx)=>{
                     return <tr key={idx} className={
                         this.state.user_name === item.name ?
                             "row-selected" : undefined}>
                              <td onClick={e=>{
                                  this.setState({user_name: item.name});
                                  this.getACL(item.name, this.state.org);
                              }}>
                                {item.name}
                              </td>
                            </tr>;
                 })}
                 </tbody>
                     </Table>
             </Container>
            </Col>
              <Col sm="4">
                <PermissionViewer
                  username={this.state.user_name}
                  org={this.state.org}
                  acls={this.state.acl}
                  setACL={this.setACL}
                />
              </Col>
            </Row>
        );
    }
}

class UserInspector extends Component {
    static contextType = UserConfig;

    static propTypes = {
        history: PropTypes.object,
    }

    state = {
        tab: "users",
        users_initialized: false,
        users: [],
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.loadUsers, POLL_TIME);
        this.loadUsers();
    }

    componentWillUnmount = () => {
        this.source.cancel();
        clearInterval(this.interval);
    }

    componentDidUpdate = (prevProps, prevState, snapshot) => {
        if (this.state.users_initialized) {
            return;
        }

        if (this.state.loading) {
            return;
        }
        this.loadUsers();
    }

    loadUsers = () => {
        this.source.cancel();
        this.source = CancelToken.source();

        this.setState({loading: true});

        api.get("v1/GetGlobalUsers", {},
                this.source.token).then((response) => {
                    if (response.cancel) return;

                    this.setState({users: response.data.users,
                                  users_initialized: true});
            });
    }

    setDefaultTab = (tab) => {
        this.setState({tab: tab});
        this.props.history.push("/users/" + tab);
    }

    render() {
        return (
            <div className="users-search-panel">
              <div className="padded">
                <Nav activeKey={this.state.tab} variant="tab"
                     onSelect={this.setDefaultTab}>
                  <Nav.Item>
                    <Nav.Link as="button" className="btn btn-default"
                              eventKey="users">{T("Users")}</Nav.Link>
                  </Nav.Item>

                  <Nav.Item>
                    <Nav.Link as="button" className="btn btn-default"
                              eventKey="orgs">{T("Orgs")}</Nav.Link>
                  </Nav.Item>
                </Nav>
                <div className="card-deck">
                  { this.state.tab === "users" &&
                    <UsersOverview
                      updateUsers={this.loadUsers}
                      users={this.state.users} />}
                  { this.state.tab === "orgs" &&
                    <OrgsOverview
                      updateUsers={this.loadUsers}
                      users={this.state.users} /> }
                </div>
              </div>
            </div>
        );
    }
}


export default withRouter(UserInspector);
