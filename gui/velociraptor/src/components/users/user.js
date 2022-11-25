import React, { Component } from 'react';
import api from '../core/api-service.js';
import _ from 'lodash';
import axios from 'axios';
import Button from 'react-bootstrap/Button';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Row';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import Tooltip from 'react-bootstrap/Tooltip';
import T from '../i8n/i8n.js';
import Card  from 'react-bootstrap/Card';
import Accordion  from 'react-bootstrap/Accordion';
import UserConfig from '../core/user.js';
import { withSnackbar } from 'react-simple-snackbar';
import "./user.css";


const roleList = [
    "administrator",
    "org_admin",
    "artifact_writer",
    "investigator",
    "analyst",
    "reader",
    "api",
];

const permissionsByRole = {
    administrator : [ "all_query", "any_query", "read_results", "label_clients",
                      "collect_client", "collect_server", "artifact_writer",
                      "server_artifact_writer", "execve", "notebook_editor", "impersonation",
                      "server_admin", "filesystem_read", "filesystem_write", "machine_state",
                      "prepare_results" ],
    org_admin : [ "org_admin" ],
    reader : [ "read_results" ],
    api : [ "any_query", "read_results" ],
    analyst : [ "read_results", "notebook_editor", "label_clients", "any_query",
                "prepare_results" ],
    investigator : [ "read_results", "notebook_editor", "collect_client", "label_clients",
                     "any_query", "prepare_results" ],
    artifact_writer : [ "artifact_writer" ]
};

const permissionNames = {
    "all_query" : "AllQuery",
    "any_query" : "AnyQuery",
    "read_results" : "ReadResults",
    "collect_client" : "CollectClient",
    "collect_server" : "CollectServer",
    "artifact_writer" : "ArtifactWriter",
    "execve" : "Execve",
    "notebook_editor" : "NotebookEditor",
    "label_clients" : "LabelClients", // Permission to apply labels to clients
    "server_admin" : "ServerAdmin",
    "filesystem_read" : "FilesystemRead",
    "filesystem_write" : "FilesystemWrite",
    "server_artifact_writer" : "ServerArtifactWriter",
    "machine_state" : "MachineState",
    "prepare_results" : "PrepareResults",
    "datastore_access" : "DatastoreAccess",
    "org_admin" : "OrgAdmin",
};

class UserState {
    static contextType = UserConfig;

    constructor () {
        this.roles = {};
        this.init_roles = {};

        this.fields = {
            'name': '',
            'email': '',
            'picture': '',
            'verified_email': false,
            // read_only exists but is unused
            'locked': false,
            'orgs': [],
            'currentOrg': '',
            'failed' : false,
        }

        this.valid_fields = {};

        this.password = '';
    }

    serialize() {
        let user = {};
        user['name'] = this.fields.name;
        user['string_fields'] = {};
        user['bool_fields'] = {};
        Object.keys(this.valid_fields).forEach((elem) => {
            switch (elem) {
            case 'email':
            case 'picture':
                    user['string_fields'][elem] = this.fields[elem];
                    break;
            case 'locked':
            case 'verified_email':
                    user['bool_fields'][elem] = this.fields[elem];
                    break;
            default:
                    console.log("Unknown field " + elem + " in serialize()");
            };
        });
        user['password'] = this.password;
        user['org_ids'] = this.fields.orgs.map((elem) => { return elem.id; });

        user['roles_per_org'] = {};

        for (const orgid in this.roles) {
            if (user['org_ids'].indexOf(orgid) !== -1) {
                user['roles_per_org'][orgid] = {strings : this.roles[orgid] };
            } else {
                user['roles_per_org'][orgid] = {strings : [] };
            }
        }

        return user;
    }

    hasRole(role, org) {
        return org in this.roles && this.roles[org].indexOf(role) !== -1;
    }

    finalize() {
        if (this.fields.orgs.length === 0) {
            this.setRootOrg();
        }
        Object.assign(this.init_roles, this.roles);
    }

    effectivePermissions(org) {
        let permissions = {};
        if (org in this.roles) {
            this.roles[org].forEach((role) => {
                permissionsByRole[role].forEach((perm) => {
                    permissions[perm] = true;
                });
            });
            if (permissions['server_admin'] && this.isMemberOf("root")) {
                permissions['org_admin'] = true;
            }
        }
        return permissions;
    }

    modifyRoles(role, org, present) {
        if (present) {
            if (!(org in this.roles)) {
                this.roles[org] = [];
            }
            this.roles[org].push(role);
        } else if (!present) {
            this.roles[org] = this.roles[org].filter((elem) => { return elem !== role });
        }
    }

    setRoles(roles, org) {
        this.roles[org] = roles;
    }

    isMemberOf(org_id) {
        for (let i = 0; i < this.fields.orgs.length; i++) {
            if (org_id === this.fields.orgs[i].id) {
                return true;
            }
        }

        return false;
    }

    setRootOrg() {
        this.fields.orgs = [ { "id" : "root", "name" : "<root>" }, ];
    }

    modifyOrgs(org, present) {
        if (!present) {
            this.fields.orgs = this.fields.orgs.filter((elem) => {
                return elem.id !== org.id;
            });
            if (this.fields.orgs.length === 0) {
                this.setRootOrg();
                return false;
            }
        } else if (present) {
            this.fields.orgs.push({id: org.id, name: org.name});
            if (!(org.id in this.roles)) {
                this.roles[org.id] = [ "reader" ];
            }
        }

        return true;
    }

    clone() {
        let user = new UserState();
        Object.assign(user.roles, this.roles);
        Object.assign(user.init_roles, this.init_roles);
        user.fields = { ...this.fields };
        user.password = this.password;
        return user
    }

    static permissionList() {
        let perms = Object.keys(permissionNames);
        return perms.sort();
    }

};

const renderToolTip = (props, value) => (
    <Tooltip show={T(value)} {...props}>
        {T(value)}
    </Tooltip>
);

//export class User extends Component {
class User extends Component {
    static contextType = UserConfig;

    state = {
        user : new UserState(),
        dirty: false,
        mode: undefined,
        currentOrg: undefined,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();

        this.maybeUpdateUser();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        if (prevProps.mode === this.props.mode && prevProps.user === this.props.user) {
            return;
        }

        this.maybeUpdateUser();
    }

    componentWillUnmount() {
      this.source.cancel("unmounted");
    }

    currentOrg() {
        return this.state.currentOrg || this.context.traits.org;
    }

    orgName(org) {
        if (org === "root") {
            return T("Organizational Root");
        }

        return this.props.orgs[org];
    }

    resetUserState() {
    }

    maybeUpdateUser() {
        if (this.props.mode === "edit" || this.props.mode === "view") {
            this.loadUser(this.props.user);
        } else {
            let user = new UserState();
            user.modifyOrgs({ name: this.context.traits.org_name, id: this.context.traits.org}, true);
            this.setState({'user' : user });
        }
    }

    handleCreate(e) {
        e.preventDefault();

        this.source.cancel();
        this.source = axios.CancelToken.source();

        let req = this.state.user.serialize();

        let endpoint = "";
        let action = "User created.";

        if (this.props.mode === 'create') {
            endpoint = "v1/CreateUser";
        } else {
            endpoint = "v1/UpdateUser";
            action = "User updated.";
        }

        api.post(endpoint, req, this.source.token).then((response) => {
            if (!response.cancel) {
                this.setState({ 'dirty' : false });
                this.props.userCreatedCallback(this.state.user.fields.name,
                                               this.state.user.fields.orgs);
                this.props.openSnackbar(T(action));
                this.passwordInput.value = '';
            }
        }, (reason) => {}); // Just to silence uncaught exceptions.  api will report the error.
    }

    loadUser(user) {
        this.source.cancel();
        this.source = axios.CancelToken.source();

        api.get("v1/GetUser/" + user, {}, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }
            let new_user_state = new UserState();

            for (const [data_key, data_value] of Object.entries(response.data)) {
                if (data_key === "Permissions") {
                    continue; // We'll get this via GetUserRoles
                } else {
                    new_user_state.fields[data_key] = data_value;
                }
            }

            let currentOrg = this.currentOrg();
            if (!new_user_state.isMemberOf(this.currentOrg())) {
                currentOrg = response.data.orgs[0].id;
            }

            this.passwordInput = '';

            return {'user' : new_user_state, 'currentOrg' : currentOrg};
        }).then ((state) =>  {
            return Promise.all([api.get("v1/GetUserRoles/" + user, {}, this.source.token), state]);
        }).then(([response, state]) => {

            for (const orgid in response.data.roles_per_org) {
                state['user'].setRoles(response.data.roles_per_org[orgid].strings, orgid);
            }

            state['user'].finalize();

            this.setState(state);
        });
    };

    changeRole(role, org, e) {
        let value = e.currentTarget.checked;
        this.setState((state) => {
            let user_state = state.user.clone();
            user_state.modifyRoles(role, org, value);
            return { 'user' : user_state, 'dirty' : true }
        })
    }

    changeOrg(org, e) {
        let value = e.currentTarget.checked;
        this.setState((state) => {
            let user_state = state.user.clone();
            if (user_state.modifyOrgs(org, value) === false) {
                    this.props.openSnackbar(T("A user without an organization defaults to membership in the root organization."));
            }
            return { 'user' : user_state, 'currentOrg' : org.id, 'dirty' : true };
        });
    }

    setUserState(name, value) {
        this.setState((state) => {
            let user_state = state.user.clone();
            user_state.fields[name] = value;
            if (name !== 'name')
                user_state.valid[name] = true;
            return { 'user' : user_state, 'dirty' : true };
        });
    }

    setUserPassword(name, value) {
        this.setState((state) => {
            let user_state = state.user.clone();
            user_state.password = value;
            return { 'user' : user_state, 'dirty' : true };
        });
    }

    wrapToolTipOverlay(wrapped, toolTipName) {
      return (
        <OverlayTrigger
          delay={{show: 250, hide: 400}} placement="right"
          overlay={(props)=>renderToolTip(props, toolTipName)}>
            <div>
            {wrapped}
            </div>
         </OverlayTrigger>
      );
    }

    renderUsernameForm() {
        let newUser = this.props.mode === 'create';
        let placeholder = T("Username");
        if (!newUser) {
            placeholder = this.props.user;
        }
        return (
          <Form.Group as={Col}>
            <Form.Label>{T("Username")}</Form.Label>
            <Form.Control placeholder={placeholder} readOnly={!newUser}
                          onChange={(e) => this.setUserState('name', e.target.value)}/>
          </Form.Group>
        );
    }

    renderUsernameField() {
        if (this.context.traits.password_less) {
            return this.wrapToolTipOverlay(this.renderUsernameForm(), "ToolUsernamePasswordless")
        }
        return this.renderUsernameForm();
    }

    renderPasswordForm() {
        let disabled = false;
        let placeholder = T("Password");
        if (this.context.traits.password_less) {
            disabled = true;
            placeholder = T("Password must be changed with authentication provider");
        }

        return (
          <Form.Group as={Col}>
            <Form.Label>{T("Password")}</Form.Label>
            <Form.Control type="password" placeholder={placeholder} disabled={disabled}
                          ref={(input) => { this.passwordInput = input; }}
                          autoComplete="new-password"
                          onChange={(e) => this.setUserPassword('password', e.target.value)}/>
          </Form.Group>
        );
    }

    renderEmailAddress() {
        return (
          <Form.Group as={Col}>
            <Form.Label>{T("Email Address")}</Form.Label>
            <Form.Control placeholder="Email" disabled={!this.props.admin}
                         onChange={(e) => this.setUserState('email', e.target.value)}
                         value={this.state.user.fields.email}/>
          </Form.Group>
        );
    }

    renderUserInfo() {
        return (
          <div className="col-sm-4">
           <Card>
            <Card.Header>
             User Information
            </Card.Header>
            <Card.Body>
             { this.renderUsernameField() }
             { this.renderPasswordForm() }
             { this.renderEmailAddress() }
             { this.renderVerifiedEmail() }
             { this.renderUserLocked() }
            </Card.Body>
           </Card>
          </div>
        )
    }

    renderRoleCardBody(org, orgName) {
        let disabled = !this.props.admin;

        return _.map(roleList, (role, idx) => {
            return (
              <OverlayTrigger key={role} delay={{show: 250, hide: 400}}
                              overlay={(props)=>renderToolTip(props, "ToolRole_" + role)}>
               <div>
                <Form.Switch label={T("Role_" + role)} id={"org_" + org + "_Roles_" + role}
                             disabled={disabled}
                             checked={this.state.user.hasRole(role, org)}
                             onChange={(e) => {this.changeRole(role, org, e)}} />
               </div>
              </OverlayTrigger>
            );
        });
    }

    renderRoleCard(orgid) {
        let orgName = this.orgName(orgid);
    let title;
    if (Object.keys(this.props.orgs).length > 1) {
        title = T("Roles in", orgName);
    } else {
        title = T("Roles");
    }
        return (
          <Card key={"RoleCard_" + orgid}>
           <Card.Header>
           <Accordion.Toggle as={Button} eventKey={orgid} variant="link">
            <OverlayTrigger delay={{show: 250, hide: 400}}
                            overlay={(props)=>renderToolTip(props, "ToolRoleBasedPermissions")}>
             <div>
               {title}
             </div>
            </OverlayTrigger>
           </Accordion.Toggle>
           </Card.Header>
          <Accordion.Collapse eventKey={orgid}>
           <Card.Body>
            { this.renderRoleCardBody(orgid, orgName) }
           </Card.Body>
           </Accordion.Collapse>
          </Card>
        );
    }

    renderEffectivePermissionsCard() {
        let org = this.currentOrg();
        let perms = this.state.user.effectivePermissions(org);
        return (
          <Card>
          <Card.Header>
            <OverlayTrigger delay={{show: 250, hide: 400}}
                            overlay={(props)=>renderToolTip(props, "ToolEffectivePermissions")}>
              <div>
                {T("Effective Permissions")}
              </div>
             </OverlayTrigger>
          </Card.Header>
            <Card.Body>
              { _.map(UserState.permissionList(), (perm, idx) => {
                    let checked = perm in perms && perms[perm];
                    return (
                      <OverlayTrigger key={perm} delay={{show: 250, hide: 400}}
                                      overlay={(props)=>renderToolTip(props, "ToolPerm_" + perm)}>
                       <div>
                        <Form.Switch disabled={true} label={permissionNames[perm]}
                                     id={perm} checked={checked} />
                       </div>
                      </OverlayTrigger>
                    );
              })}
            </Card.Body>
          </Card>
        );
    }

    selectOrg(e) {
        if (e) {
            this.setState({'currentOrg' : e});
        }
    }

    renderPermissions() {
        let orgs = Object.keys(this.props.orgs);
        orgs.sort((first, second) => {
            return orgs[first] > orgs[second];
        });

        let activeKey = this.currentOrg();
        let member = this.state.user.isMemberOf(this.currentOrg());
        if (!member) {
            let valid = orgs.filter((elem) => { return this.state.user.isMemberOf(elem); })
            activeKey = valid[0];
        }

        return (
          <>
           <div className="col-sm-4">
            <Accordion onSelect={(e) => this.selectOrg(e)} activeKey={activeKey}>
              { _.map(orgs, (orgid, idx) => {
                if (this.state.user.isMemberOf(orgid)) {
                    return this.renderRoleCard(orgid);
                }
              })}
            </Accordion>
            { this.renderEffectivePermissionsCard(activeKey) }
           </div>
          </>
        );
    }

    renderOrganizationsCard() {
        let orgs = Object.keys(this.props.orgs);
        orgs.sort((first, second) => {
            return orgs[first] > orgs[second];
        });
        return (
          <Card>
            <Card.Header>
              <OverlayTrigger delay={{show: 250, hide: 400}}
                              overlay={(props)=>renderToolTip(props, "ToolOrganizations")}>
                <div>
                  {T("Organizations")}
                </div>
              </OverlayTrigger>
            </Card.Header>
            <Card.Body>
              { _.map(orgs, (orgid, idx) => {
                    let org = { 'id': orgid, 'name' : this.orgName(orgid) };
                    let name = org.name;
                    let checked = this.state.user.isMemberOf(org.id);
                    let key = org.id;
                    if (org.id === "root") {
                        // We can't have an id of 'root'
                        key = "Org_" + org.id;
                        name = T("Organizational Root");
                    }
                    return (
                       <Form.Switch label={name} id={key} key={key} checked={checked}
                                    onChange={(e) => {this.changeOrg(org, e)}} />
                    );
              })}
            </Card.Body>
          </Card>

        );
    }

    renderOrganizations() {
        return (
          <>
           <div className="col-sm-4">
            { this.renderOrganizationsCard() }
           </div>
          </>
        );
    }

    renderVerifiedEmail() {
        let disabled = this.state.user.fields.email === '' || !this.props.admin;
        let checked = this.state.user.fields.verified_email;

        return (
          <Form.Group>
            <OverlayTrigger
             delay={{show:250, hide: 400}}
             overlay={(props)=>renderToolTip(props, "ToolUser_verified_email")}>
             <div>
              <Form.Switch label={T("Verified Email")} id="verified-email"
                           disabled={disabled} checked={checked}
                           onChange={(e) => { this.setUserState('verified_email',
                                                                e.currentTarget.checked)}} />
             </div>
            </OverlayTrigger>
          </Form.Group>
        );
    }

    renderUserLocked() {
        let disabled = !this.props.admin;
        let checked = this.state.user.fields.locked;
        return (
          <Form.Group>
            <OverlayTrigger
             delay={{show:250, hide: 400}}
             overlay={(props)=>renderToolTip(props, "ToolUser_locked")}>
             <div>
              <Form.Switch label={T("Account Locked")} id="locked"
                           disabled={disabled} checked={checked}
                           onChange={(e) => {this.setUserState('locked',
                                                               e.currentTarget.checked)}} />
             </div>
            </OverlayTrigger>
          </Form.Group>
        );
    }

    renderUserFlags() {
        return (
          <div className="col-sm-4">
           { this.renderVerifiedEmail() }
           { this.renderUserLocked() }
          </div>
        );
    }

    renderSubmitButton() {
        if (!this.props.admin) {
            return;
        }

        return (
          <div className="col-sm-4">
           <Button onClick={(e) => this.handleCreate(e)} disabled={!this.state.dirty}
                   variant="default" type="button">
             {this.props.mode === "create" ? "Add User" : "Update User"}
           </Button>
          </div>
        );
    }

    render() {
        if (!this.context.traits.username) {
            return null;
        }
        let header = "";
        if (this.props.mode === "create") {
            if (!this.props.admin) {
                return null;
            }
            header = T("Create a new user");
        } else {
            if (!this.props.admin) {
                header = T("User Information");
            } else {
                header = T("Edit User");
            }
        }

        return(
          <div className="user">
           <h2 className="mx-1">{header}</h2>
           <Form onSubmit={(e) => e.preventDefault()} ref={ form => this.userForm = form }>
            <div className="row ml-3">
             { this.renderUserInfo() }
             { Object.keys(this.props.orgs).length > 1 && this.renderOrganizations() }
             { this.renderPermissions() }
            </div>
            <div className="row ml-3">
             { this.renderSubmitButton() }
            </div>
           </Form>
          </div>
        );
    }
};

export default withSnackbar(User);
