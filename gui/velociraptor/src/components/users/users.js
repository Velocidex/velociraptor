import React, { Component } from 'react';
import api from '../core/api-service.js';
import Alert from 'react-bootstrap/Alert';
import axios from 'axios';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import User from './user.js';
import T from '../i8n/i8n.js';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import InputGroup from 'react-bootstrap/InputGroup';
import SplitPane from 'react-split-pane';
import Navbar from 'react-bootstrap/Navbar';
import Table  from 'react-bootstrap/Table';
import Form from 'react-bootstrap/Form';
import FormControl from 'react-bootstrap/FormControl';
import Container from  'react-bootstrap/Container';
import VeloTable from '../core/table.js';
import UserConfig from '../core/user.js';
import Spinner from '../utils/spinner.js';

import './users.css';

//export default class Users extends Component {
class Users extends Component {
    static contextType = UserConfig;

    state = {
        fullSelecteddescriptor: {},
        matchingDescriptors: [],
        timeout: 0,
        current_filter: "",
        nameFilter: "",
        filteredUsers: [],
        allUsers: {},
        explicitFilter: true,
        updatePending: false,
        showUserTable: false,

        users_initialized: false,
        roles_initialized: false,
        loading: false,
        context_found: false,
    }

    constructor(props, context) {
        super(props, context);
        let username = props.match && props.match.params && props.match.params.user;
        if (username) {
            this.state.explicitFilter = false;
        }
    }

    componentDidMount = () => {
        this.usersSource = axios.CancelToken.source();
        this.orgsSource = axios.CancelToken.source();
        if (this.nameInput) {
            this.nameInput.focus();
        }
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        if (prevProps === this.props && prevState === this.state && this.state.initialized) {
            return;
        }

        if (this.state.users_initialized) {
            return;
        }

        if (this.state.loading) {
            return;
        }

        if (!prevState.loading && this.context.traits.username) {
            this.loadUsers();
        }
    }

    filterUsers(allUsers, nameFilter) {
        let users = [];

        for (const username in allUsers) {
            if (!username.startsWith(nameFilter)) {
                continue;
            }
            users.push(username);
        }

        return users.sort();
    }

    clearFilter(e) {
        let users = this.filterUsers(this.state.allUsers, "", "");
        this.setState({nameFilter: "", filteredUsers: users});
    }

    updateNameFilter(e, value) {
        e.preventDefault();
        if (value !== this.state.nameFilter) {
            let users = this.filterUsers(this.state.allUsers, value);
            this.setState({nameFilter: value, filteredUsers: users, explicitFilter: true});
        }
    }

    isAdminUser() {
        let perms = this.context.traits.Permissions;
        return perms && perms.server_admin;
    }

    handleDelete(e, user) {
        e.preventDefault();

        if (!window.confirm(T("Do you want to delete?", user))) {
            return;
        }

        api.delete_req("v1/DeleteUser/" + user, {}, this.usersSource.token).then((response) => {;
            let names = {...this.state.allUsers};
            delete names[user]

            let filtered = this.state.filteredUsers.filter((elem) => { return user !== elem; });

            this.setState({allUsers: names, filteredUsers: filtered});
            this.props.history.push("/users/");
        });
    }

    // This will load all of the orgs that are visible to us.  Since we can't
    // be an admin of an org without belonging to it, any user record with
    // orgs assigned to it will provide a list of all of the orgs we can
    // administer.
    loadUsers() {
        if (!this.isAdminUser() || this.state.users_initialized) {
            return;
        }
        this.usersSource.cancel()
        this.usersSource = axios.CancelToken.source();

        this.setState({loading: true});

        api.get("v1/GetGlobalUsers", {}, this.usersSource.token).then((response) => {
            if (response.cancel)
                return;

            let orgNames = {};
            let names = {};
            let state = {};

            response.data.users.forEach((user) => {
                if (!user.orgs) {
                    return;
                }

                names[user.name] = { name: user.name,
                                     orgs: user.orgs }
                user.orgs.forEach((org) => {
                    orgNames[org.id] = org.name;
                });
            });

            let orgs = [];
            for (const [id, name] of Object.entries(orgNames)) {
                orgs.push({'id': id, 'name': name});
            }

            let tableData = {
                columns: [ "user", "OrgRoles" ],
                rows: response.data.users,
            };

            state['allUsers'] = names;
            state['filteredUsers'] = Object.keys(names).sort()
            state['users_initialized'] = true;
            state['tableHeaders'] = { 'user' : T("User"), 'OrgRoles' : T("Roles per Organization") };
            state['tableData'] = tableData;
            state['loading'] = false;
            state['orgs'] = orgs;
            state['orgNames'] = orgNames;

            this.setState(state);
        });
    }

    onSelect(username, e) {
        this.setState({ createUser: false });
        this.props.history.push("/users/" + username);
        e.preventDefault();
        e.stopPropagation();

        return false;
    }

    clickNewUser(e) {
        this.setState({createUser: true, updatePending: true});
        this.props.history.push("/users/");
        e.preventDefault();
        e.stopPropagation();

        return false;
    }

    userCreatedCallback(username, orgs) {
        let newState = { allUsers : {...this.state.allUsers } };
        newState.allUsers[username] = { 'name': username, 'orgs': orgs };
        newState.filteredUsers = this.filterUsers(newState.allUsers, this.state.nameFilter);
        this.setState(newState);
        this.props.history.push("/users/" + username);
    }

    renderUserPanel() {
        let mode = undefined;
        let username = this.props.match && this.props.match.params && this.props.match.params.user;

        if (!this.state.users_initialized) {
            return null;
        }

        if (username && !this.state.allUsers[username]) {
            return <Alert variant="warning">{T("User does not exist", username)}</Alert>;
        }

        if (this.state.createUser) {
            mode = "create";
        } else if (username) {
            mode = "edit"
        }

        if (mode) {
            return (
              <User mode={mode} user={username} admin={this.isAdminUser()}
                    orgs={this.state.orgNames}
                    userCreatedCallback={(username, orgs) => {this.userCreatedCallback(username, orgs)}}/>);
        } else {
            return (<h2 className="no-content">{T("Select a user to view it")}</h2>);
        }
    }

    renderUserRow(user) {
        let selected = user === this.props.match.params.user;

        return (
          <tr key={user} className={ selected ? "row-selected" : undefined }>
             <td onClick={(e) => this.onSelect(user, e)}>
               {/* eslint jsx-a11y/anchor-is-valid: "off" */}
               <a href="#" onClick={(e) => this.onSelect(user, e)}>{user}</a>
             </td>
          </tr>
        );
    }

    renderUserSelector() {
        let orgUsers = [];
        let otherUsers = [];

        if (this.state.filteredUsers.length === 0) {
            return;
        }

    if (this.state.orgs.length > 1) {
            this.state.filteredUsers.forEach((username) => {
                let user = this.state.allUsers[username];
                for (let i = 0; i < user.orgs.length; i++) {
                    if (user.orgs[i].id === this.context.traits.org) {
                        orgUsers.push(username);
                        return;
                    }
                }
                otherUsers.push(username);
            });
        } else {
            orgUsers = this.state.filteredUsers;
        }

        return (
          <>
            <Table bordered hover size="sm">
              <tbody>
                {this.state.orgs.length > 1 && <tr key="current_org_row">
                    <th>{this.context.traits.org_name}</th>
                </tr> }
                { orgUsers.map((username, idx) => {
                    return this.renderUserRow(username);
                })}
              </tbody>
              <tbody>
                {otherUsers.length > 0 && <tr key="other_org_row">
                    <th>{T("Users in other organizations")}</th>
                </tr> }
                { otherUsers.map((username, idx) => {
                    return this.renderUserRow(username);
                })}
              </tbody>
            </Table>
          </>
        );
    }

    renderCreate() {
        return (
          <>
            <Button title={T("Add New User")}
                    onClick={(e) => this.clickNewUser(e)} variant="default">
              <FontAwesomeIcon icon="plus"/>
            </Button>
          </>
        );
    }

    renderDelete() {
        if (this.props.match.params.user && !this.state.createUser) {
            return (
              <>
                <Button title={T("Delete")}
                            onClick={(e) => this.handleDelete(e, this.props.match.params.user)}
                            variant="default">
                  <FontAwesomeIcon icon="window-close"/>
                </Button>
              </>
            );
        }
    }

    renderLoadingSpinner() {
        if (this.state.loading) {
            return <FontAwesomeIcon icon="spinner" spin/>
        } else {
            return <FontAwesomeIcon icon="broom"/>
        }
    }

    renderSearchNavbar() {
        if (this.state.showUserTable || !this.isAdminUser()) {
            return;
        }
        return (
          <Form inline className="">
            <InputGroup >
              <InputGroup.Prepend>
                <Button variant="outline-default"
                        onClick={() => this.clearFilter()}
                        size="sm">
                  { this.renderLoadingSpinner() }
                </Button>
              </InputGroup.Prepend>
              <FormControl size="sm"
                           ref={(input) => { this.nameInput = input; }}
                           value={this.state.explicitFilter ? this.state.nameFilter : ""}
                           onChange={(e) => this.updateNameFilter(e, e.currentTarget.value)}
                           placeholder={T("Search for user")}
                           spellCheck="false"/>
            </InputGroup>
          </Form>
        );
    }

    renderTableToggle() {
        if (this.state.showUserTable) {
            return (
              <Button title={T("Show user selector")}
                          onClick={() => this.setState({showUserTable: false})}
                          variant="default">
                <FontAwesomeIcon icon="list"/>
              </Button>
            );
      }
      return (
        <Button title={T("Show user tables")}
                    onClick={() => this.setState({showUserTable: true})}
                    variant="default">
          <FontAwesomeIcon icon="binoculars"/>
        </Button>
      );
    }

    renderUnprivilegedNavbar() {
        return (
          <Navbar className="justify-content-between">
            <ButtonGroup>
              { this.renderTableToggle() }
            </ButtonGroup>
          </Navbar>
        );
    }


    renderAdminNavbar() {
        return (
          <Navbar className="justify-content-between">
            <ButtonGroup>
              { this.renderCreate() }
              { this.renderTableToggle() }
              { this.renderDelete() }
            </ButtonGroup>
            { this.renderSearchNavbar() }
          </Navbar>
        );
    }

    renderNavbar() {
        if (this.isAdminUser()) {
            return this.renderAdminNavbar();
        }

        return this.renderUnprivilegedNavbar();
    }

    renderOneUser() {
        return (
          <>
            { this.renderUnprivilegedNavbar() }
            <User mode={"view"} user={this.context.traits.username}
                  password_less={this.context.traits.password_less} />
          </>
        );
    }

    render() {
        if (!this.context.traits.username) {
            return <Spinner loading={!this.state.users_initialized} />;
        }
        if (this.state.showUserTable) {
            if (!this.state.tableData) {
                return <Spinner loading={!this.state.tableData} />;
            }
            return (
              <>
                { this.renderNavbar() }
                <div className="users-user-table">
                <VeloTable
                  className="col-12"
                  rows={this.state.tableData.rows}
                  headers={this.state.tableHeaders}
                  columns={this.state.tableData.columns} />
                </div>
              </>
            );
        }

        if (!this.isAdminUser()) {
            return this.renderOneUser();
        }

        return (
          <>
            { this.renderNavbar() }
            <div className="users-search-panel">
              <Spinner loading={!this.state.users_initialized} />
              <SplitPane split="vertical" defaultSize="70%">
                <div className="users-search-results">
                  { this.renderUserPanel() }
                </div>
                <Container className="user-search-table selectable">
                  { this.renderUserSelector() }
                </Container>
                </SplitPane>
            </div>
          </>
        );
    }
};
