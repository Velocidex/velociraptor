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

import "./secrets.css";

const POLL_TIME = 5000;

class AddSecretTypeDialog extends Component {
    static propTypes = {
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount = () => {
        this.source.cancel();
    }

    addSecretDefinition = () => {
        this.source.cancel();
        this.source = CancelToken.source();

        api.post("v1/DefineSecret", {
            type_name: this.state.type_name,
            verifier: this.state.verifier,
        },  this.source.token).then(response=>{
            if (response.cancel)
                return;
            this.props.onClose();
        });
    }

    state = {
        type_name: "",
        verifier: "",
    }

    render() {
        return(
            <Modal show={true}
                   size="lg"
                   enforceFocus={true}
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Warning")}</Modal.Title>
              </Modal.Header>
              <Modal.Body >
                {T("Add Secret Type")}
                 <VeloForm
                   param={{name: T("Secret Type"), description: T("Type of secret to add")}}
                   value={this.state.type_name}
                   setValue={x=>this.setState({type_name:x})}
                 />
                 <VeloForm
                   param={{name: T("Secret Verifier"), description: T("A VQL Lambda to verify the secret")}}
                   value={this.state.verifier}
                   setValue={x=>this.setState({verifier:x})}
                 />

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        disabled={!this.state.type_name}
                        onClick={this.addSecretDefinition}>
                  {T("Do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

class EditSecretDialog extends Component {
    static propTypes = {
        secret: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.setState({new_users: this.props.secret.users});
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
        return(
            <Modal show={true}
                   size="lg"
                   enforceFocus={true}
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Warning")}</Modal.Title>
              </Modal.Header>
              <Modal.Body >
                <h1>{T("Edit Secret")} { this.state.secret.name } </h1>
                {T("Share secret with these users")}
                <UserForm
                  value={this.state.new_users}
                  onChange={users=>this.setState({new_users: users})}/>

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
    }

    render() {
        return(
            <Modal show={true}
                   size="lg"
                   enforceFocus={true}
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Warning")}</Modal.Title>
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
        secret: PropTypes.array,
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.setState({secret: [...this.props.secret]});
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
                <Modal.Title>{T("Warning")}</Modal.Title>
              </Modal.Header>
              <Modal.Body >
                {T("Add New Secret")}
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
        let res = [];
        _.map(this.state.current_definition.template || {}, (y, x)=>{
            res.push([x,y]);
        });
        this.setState({current_secret: res});
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

    render() {
        return (
            <Row className="secret-manager">
              { this.state.showAddSecretTypeDialog &&
                <AddSecretTypeDialog
                  onClose={()=>{
                      this.setState({showAddSecretTypeDialog: false});
                      this.getSecretDefinitions();
                  }}
                />
              }
              { this.state.showAddSecretDialog &&
                <AddSecretDialog
                  secret={this.state.current_secret}
                  type_name={this.state.current_definition.type_name}
                  verifier={this.state.current_definition.verifier}
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
                          <Button
                            data-tooltip={T("Add new type")}
                            data-position="left"
                            onClick={()=>this.setState({
                                showAddSecretTypeDialog: true
                            })}
                            className="btn-tooltip new-user-btn"
                            variant="outline-default"
                            as="button">
                            <FontAwesomeIcon icon="plus"/>
                            <span className="sr-only">
                              {T("Add new type")}
                            </span>
                          </Button>
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
                            <Button
                              data-tooltip={T("Add new secret")}
                              data-position="left"
                              onClick={()=>{
                                  this.setState({showAddSecretDialog: true});
                                  this.updateTemplate();
                              }}
                              className="btn-tooltip new-user-btn"
                              variant="outline-default"
                              as="button">
                              <FontAwesomeIcon icon="plus"/>
                              <span className="sr-only">
                                {T("Add new secret")}
                              </span>
                            </Button>
                            { this.state.current_secret.name && <>
                              <Button
                                data-tooltip={T("Edit secret")}
                                data-position="left"
                                onClick={()=>this.setState({
                                    showEditSecretDialog: true
                                })}
                                className="btn-tooltip new-user-btn"
                                variant="outline-default"
                                as="button">
                                <FontAwesomeIcon icon="edit"/>
                                <span className="sr-only">
                                  {T("Edit secret")}
                                </span>
                              </Button>
                              <Button
                                data-tooltip={T("Delete secret")}
                                data-position="left"
                                onClick={()=>this.setState({
                                    showDeleteSecretDialog: true
                                })}
                                className="btn-tooltip new-user-btn"
                                variant="outline-default"
                                as="button">
                                <FontAwesomeIcon icon="trash"/>
                                <span className="sr-only">
                                  {T("Delete secret")}
                                </span>
                              </Button> </>
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
              { this.state.current_secret.name &&
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
