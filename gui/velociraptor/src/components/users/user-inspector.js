import _ from 'lodash';

import "./user.css";

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';
import T from '../i8n/i8n.js';
import { withRouter }  from "react-router-dom";
import UserConfig from '../core/user.js';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Container from  'react-bootstrap/Container';
import Table  from 'react-bootstrap/Table';
import Card  from 'react-bootstrap/Card';
import Button from 'react-bootstrap/Button';
import Form from 'react-bootstrap/Form';
import Modal from 'react-bootstrap/Modal';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import api from '../core/api-service.js';
import axios from 'axios';

const POLL_TIME = 5000;


class PermissionViewer extends Component {
    static propTypes = {
        acls: PropTypes.object,
        setACL: PropTypes.func.isRequired,
    }

    state = {
        showDialog: false,
        message: "",
        help_topic: ""
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
        if (_.size(this.props.acls) === 0) {
           return <></>;
        }

        let org_name = this.props.acls.org_name || "root";

        return (
            <Container className="permission-viewer">
              <Modal show={this.state.showDialog}
                     onHide={() => this.setState({showDialog: false})}>
                <Modal.Header closeButton>
                  <Modal.Title>{T(this.state.help_topic)}</Modal.Title>
                </Modal.Header>
                <Modal.Body >
                  {T(this.state.message)}
                </Modal.Body>
              </Modal>

              <Card>
                <Card.Header>
                  {T("Roles")} - {this.props.acls.name} @ {org_name}
                </Card.Header>
                <Card.Body>
                  { _.map(this.props.acls.all_roles, (role, idx)=>(
                        <div className="form-toggle" key={role}>
                          <Button
                            variant="outline-default"
                            as="a"
                            onClick={()=>this.setState({
                                showDialog: true,
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
                            as="a"
                            onClick={()=>this.setState({
                                showDialog: true,
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
                          as="a"
                          onClick={()=>this.setState({
                              showDialog: true,
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
    static propTypes = {
        users: PropTypes.array,
    }

    state = {
        user: "",
        org: "",
        acl: {},
    }

    componentDidMount = () => {
        this.setACLsource = axios.CancelToken.source();
        this.getACLsource = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.setACLsource.cancel();
        this.getACLsource.cancel();
    }


    setACL = (acl) => {
        this.setACLsource.cancel();
        this.setACLsource = axios.CancelToken.source();

        api.post("v1/SetUserRoles", acl,
                 this.setACLsource.token).then(response=>{
                     if (response.cancel)
                         return;
                     this.getACL({name: acl.name},
                                 {id: acl.org, name: acl.org_name});
                 });
    }

    getACL = (user, org) => {
        this.getACLsource.cancel();
        this.getACLsource = axios.CancelToken.source();

        this.setState({acl: {}});
        let user_name = user && user.name;
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
        let selected_orgs = this.state.user && this.state.user.orgs;

        return (
            <Row>
              <Col sm="4">
                  <Container className="selectable">
                    <Table  bordered hover size="sm">
                      <thead>
                        <tr><th>{T("Users")}</th></tr>
                      </thead>
                      <tbody>
                        { _.map(this.props.users, (item, idx)=>{
                            return <tr key={idx} className={
                                this.state.user &&
                                    this.state.user.name === item.name ?
                                    "row-selected" : undefined
                                }>
                                     <td onClick={e=>{
                                         this.setState({user: item});
                                         this.getACL(item, this.state.org);
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
                  <Container className="selectable">
                    <Table  bordered hover size="sm">
                      <thead>
                        <tr><th>{T("Orgs")}</th></tr>
                      </thead>
                      <tbody>
                        { _.map(selected_orgs, (item, idx)=>{
                            return <tr key={idx} className={
                                this.state.org &&
                                    this.state.org.name === item.name ?
                                    "row-selected" : undefined}>
                                     <td onClick={e=>{
                                         this.setState({org: item});
                                         this.getACL(this.state.user, item);
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
                   <Container className="selectable">
                    <Table  bordered hover size="sm">
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
                                              {id: org_id_by_name[name]},
                                              this.state.user);
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
            <Container className="selectable">
             <Table  bordered hover size="sm">
                     <thead>
                       <tr><th>{T("Users")}</th></tr>
                     </thead>
                     <tbody>
                 { _.map(selected_users, (item, idx)=>{
                     return <tr key={idx} className={
                         this.state.user &&
                             this.state.user.name === item.name ?
                             "row-selected" : undefined}>
                              <td onClick={e=>{
                                  this.setState({user: item});
                                  this.getACL(item, this.state.org);
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

    state = {
        tab: "users",
        users_initialized: false,
        users: [],
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
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
        this.source = axios.CancelToken.source();

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
                <Tabs activeKey={this.state.tab}
                      onSelect={this.setDefaultTab}>
                  <Tab eventKey="users" title={T("Users")}>
                    { this.state.tab === "users" &&
                      <UsersOverview users={this.state.users} />}
                  </Tab>
                  <Tab eventKey="orgs" title={T("Orgs")}>
                    { this.state.tab === "orgs" &&
                      <OrgsOverview users={this.state.users} /> }
                  </Tab>
                </Tabs>
              </div>
            </div>
        );
    }
}


export default withRouter(UserInspector);
