import _ from 'lodash';

import PropTypes from 'prop-types';

import React, { Component } from 'react';
import Col from 'react-bootstrap/Col';
import Container from  'react-bootstrap/Container';
import Table  from 'react-bootstrap/Table';
import Button from 'react-bootstrap/Button';
import T from '../i8n/i8n.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import Modal from 'react-bootstrap/Modal';
import VeloForm from '../forms/form.jsx';
import Row from 'react-bootstrap/Row';
import DictEditor from '../forms/dict.jsx';
import UserForm from '../utils/users.jsx';
import Form from 'react-bootstrap/Form';
import ToolTip from '../widgets/tooltip.jsx';

import "./secrets.css";

const POLL_TIME = 5000;

class EditSecretDialog extends Component {
    static propTypes = {
        secret: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.setState({
            new_users: this.props.secret.users,
            visible_to_all_orgs: this.props.secret.visible_to_all_orgs});
    }

    componentWillUnmount = () => {
        this.source.cancel();
    }

    addUsers = ()=>{
        this.source.cancel();
        this.source = CancelToken.source();

       if(!this.props.secret) {
            return;
        }

        api.post("v1/ModifySecret", {
            type_name: this.props.secret.type_name,
            name: this.props.secret.name,
            add_users: this.state.new_users,
            visible_to_all_orgs: this.state.visible_to_all_orgs,
            remove_users: this.props.secret.users,
        },  this.source.token).then(response=>{
            if (response.cancel)
                return;

            this.props.onClose();
        });
    }

    state = {
        secret: {},
        new_users: [],
    }

    render() {
        let org_id = window.globals.OrgId || "root";

        return(
            <Modal show={true}
                   size="lg"
                   enforceFocus={true}
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Edit Secret properties")}</Modal.Title>
              </Modal.Header>
              <Modal.Body >
                <h1>{T("Edit Secret")} { this.state.secret.name } </h1>
                {T("Share secret with these users")}
                <UserForm
                  includeSuperuser={true}
                  value={this.state.new_users}
                  onChange={users=>this.setState({new_users: users})}/>
                { org_id == "root" &&
                  <VeloForm
                    value={this.state.visible_to_all_orgs}
                    setValue={x=>this.setState(
                        {visible_to_all_orgs: x==="Y"})}
                    param={{
                        name: T("Visible To All Orgs"),
                        description: T("Make the secret visible to all sub orgs"),
                        type: "bool",
                    }}/>
                }

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        onClick={this.addUsers}>
                  {T("Do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

class DeleteSecretDialog extends Component {
    static propTypes = {
        secret: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount = () => {
        this.source.cancel();
    }

    deleteSecret = ()=>{
        this.source.cancel();
        this.source = CancelToken.source();

        if(!this.props.secret) {
            return;
        }

        api.post("v1/ModifySecret", {
            type_name: this.props.secret.type_name,
            name: this.props.secret.name,
            delete: true,
        },  this.source.token).then(response=>{
            if (response.cancel)
                return;

            this.props.onClose();
        });
    }

    state = {
        secret: {},
        previous_users: [],
        new_users: [],
        visible_to_all_orgs: false,
    }

    render() {
        return(
            <Modal show={true}
                   size="lg"
                   enforceFocus={true}
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Delete secret")}</Modal.Title>
              </Modal.Header>
              <Modal.Body >
                <h1>{this.props.secret.name }</h1>
                {T("This secret will be permanently deleted")}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        onClick={this.deleteSecret}>
                  {T("Do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

class AddSecretDialog extends Component {
    static propTypes = {
        type_name: PropTypes.string,
        verifier: PropTypes.string,
        description: PropTypes.string,
        secret: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        let secret_array = [];
        _.map(this.props.secret, (v,k)=>{
            secret_array.push([k, v]);
        });

        this.setState({secret: secret_array});
    }

    componentWillUnmount = () => {
        this.source.cancel();
    }

    addSecret = () => {
        this.source.cancel();
        this.source = CancelToken.source();

        let secret = {};
        _.map(this.state.secret, x=>{
            secret[x[0]]= x[1];
        });

        api.post("v1/AddSecret", {
            name: this.state.name,
            type_name: this.props.type_name,
            secret: secret,
        },  this.source.token).then(response=>{
            if (response.cancel)
                return;
            this.props.onClose();
        });
    }

    state = {
        name: "",
        verifier: "",
        secret: [],
    }

    render() {
        return(
            <Modal show={true}
                   size="lg"
                   enforceFocus={true}
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Add secret from template: ")}
                  {this.props.type_name}</Modal.Title>
              </Modal.Header>
              <Modal.Body  className="new-secret-dialog">
                <p>
                  {this.props.description}
                </p>
                 <VeloForm
                   param={{name: T("Secret Name"), description: T("The name of the secret to add")}}
                   value={this.state.name}
                   setValue={x=>this.setState({name:x})}
                 />
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    {T("Verify Expression")}
                  </Form.Label>
                  <Col sm="8">
                    { this.props.verifier }
                  </Col>
                </Form.Group>

                <DictEditor
                  value={this.state.secret}
                  setValue={x=>this.setState({secret: x})}
                />

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        disabled={!this.state.name}
                        onClick={this.addSecret}>
                  {T("Do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


export default class SecretManager extends Component {
    state = {
        secret_definitions: [],
        current_definition: {},
        current_secret: {},
        current_secret_names: [],

        AddSecretTypeDialog: false,
        AddSecretDialog: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.getSecretDefinitions, POLL_TIME);
        this.getSecretDefinitions();
    }

    componentWillUnmount = () => {
        this.source.cancel();
        clearInterval(this.interval);
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        return _.isEqual(prevState.current_definition,
                         this.state.current_definition);
    }

    getSecretDefinitions = () => {
        this.source.cancel();
        this.source = CancelToken.source();

        api.get("v1/GetSecretDefinitions", {},
                this.source.token).then(response=>{
                    if (response.cancel)
                        return;
                    this.setState({
                        secret_definitions: response.data.items || []
                    });

                    if(this.state.current_definition.type_name) {
                        let found = _.filter(response.data.items,
                                             x=>x.type_name === this.state.current_definition.type_name);
                        if(found.length === 0) {
                            this.setState({current_definition: {}, current_secret: {}});
                            return;
                        }

                        this.setState({current_definition: found[0]});

                        // Check if the current highlighted secret is
                        // still valid (it might have been deleted).
                        let found_secret = _.filter( found[0].secret_names,
                                name=>this.state.current_secret.name === name);
                        if(found_secret.length === 0) {
                            this.setState({current_secret: {}});
                            return;
                        }
                    }

                    if(this.state.current_secret.name) {
                        this.fetchSecret(this.state.current_secret.type_name,
                                         this.state.current_secret.name);
                    }
                });
    }

    updateTemplate = ()=>{
        let template = {};

        // Maintain the order of the fields.
        _.each(this.state.current_definition.fields, x=>{
            template[x] = this.state.current_definition.template[x] || "";
        });

        this.setState({
            current_secret: template});
    }

    fetchSecret = (type_name, name) => {
        this.source.cancel();
        this.source = CancelToken.source();

        api.get("v1/GetSecret", {
            type_name: type_name,
            name: name,
        },  this.source.token).then(response=>{
            if (response.cancel)
                return;

            this.setState({
                current_secret: response.data,
            });
        });
    }

    getItemIcon = item=>{
        if (item.built_in) {
            return <FontAwesomeIcon icon="house" /> ;
        }
        return <FontAwesomeIcon icon="user-edit" /> ;
    };

    render() {
        return (
            <Row className="secret-manager">
              { this.state.showAddSecretDialog &&
                <AddSecretDialog
                  secret={this.state.current_secret}
                  type_name={this.state.current_definition.type_name}
                  verifier={this.state.current_definition.verifier}
                  description={this.state.current_definition.description || ""}
                  onClose={()=>{
                      this.setState({showAddSecretDialog: false});
                      this.getSecretDefinitions();
                  }}
                />
              }

              { this.state.showEditSecretDialog &&
                <EditSecretDialog
                  secret={this.state.current_secret}
                  onClose={()=>{
                      this.setState({showEditSecretDialog: false});
                      this.getSecretDefinitions();
                  }}
                />
              }
              { this.state.showDeleteSecretDialog &&
                <DeleteSecretDialog
                  secret={this.state.current_secret}
                  onClose={()=>{
                      this.setState({showDeleteSecretDialog: false});
                      this.getSecretDefinitions();
                  }}
                />
              }

              <Col sm="4">
                <Container className="selectable user-list">
                  <Table  bordered hover size="sm">
                    <thead>
                      <tr>
                        <th>
                          {T("Secret Types")}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      { _.map(this.state.secret_definitions, (item, idx)=>{
                          return <tr key={idx} className={
                              this.state.current_definition.type_name === item.type_name ?
                                  "row-selected" : undefined
                          }>
                                   <td onClick={e=>{
                                       this.setState({
                                           current_secret: {},
                                           current_definition: item});
                                   }}>
                                     <span className="built-in-icon">
                                       {this.getItemIcon(item)}
                                     </span>
                                     {item.type_name}
                                   </td>
                                 </tr>;
                      })}
                    </tbody>
                  </Table>
                </Container>
              </Col>
              { this.state.current_definition.type_name &&
                <Col sm="4">
                  <Container className="selectable user-list">
                    <Table  bordered hover size="sm">
                      <thead>
                        <tr>
                          <th>
                            {T("Secret Names")}
                            <ToolTip tooltip={T("Add new secret")}>
                              <Button
                                onClick={()=>{
                                    this.setState({showAddSecretDialog: true});
                                    this.updateTemplate();
                                }}
                                className="new-user-btn"
                                variant="outline-default"
                                as="button">
                                <FontAwesomeIcon icon="plus"/>
                                <span className="sr-only">
                                  {T("Add new secret")}
                                </span>
                              </Button>
                            </ToolTip>
                            { this.state.current_secret &&
                              this.state.current_secret.name &&
                              <>
                                <ToolTip tooltip={T("Edit secret")}>
                                  <Button
                                    onClick={()=>this.setState({
                                        showEditSecretDialog: true
                                    })}
                                    className="new-user-btn"
                                    variant="outline-default"
                                    as="button">
                                    <FontAwesomeIcon icon="edit"/>
                                    <span className="sr-only">
                                      {T("Edit secret")}
                                    </span>
                                  </Button>
                                </ToolTip>
                                <ToolTip tooltip={T("Delete secret")}>
                                  <Button
                                    onClick={()=>this.setState({
                                        showDeleteSecretDialog: true
                                    })}
                                    className="new-user-btn"
                                    variant="outline-default"
                                    as="button">
                                    <FontAwesomeIcon icon="trash"/>
                                    <span className="sr-only">
                                      {T("Delete secret")}
                                    </span>
                                  </Button>
                                </ToolTip>
                              </>
                            }
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        { _.map(this.state.current_definition.secret_names, (name, idx)=>{
                            return <tr key={idx} className={
                                this.state.current_secret.name === name ?
                                    "row-selected" : undefined
                            }>
                                     <td onClick={e=>{
                                         this.fetchSecret(
                                             this.state.current_definition.type_name, name);
                                     }}>
                                       {name}
                                     </td>
                                   </tr>;
                        })}
                      </tbody>
                    </Table>
                  </Container>
                </Col>
              }
              { this.state.current_secret && this.state.current_secret.name &&
                <Col sm="3">
                  <Container className="selectable user-list">
                    <Table  bordered hover size="sm">
                      <thead>
                        <tr>
                          <th>
                            {T("Permitted Users")}
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        { _.map(this.state.current_secret.users, (name, idx)=>{
                            return  <tr key={idx}><td>{name}</td></tr>;
                        })}
                      </tbody>
                    </Table>
                  </Container>
                </Col>
              }
            </Row>
        );
    }
}
