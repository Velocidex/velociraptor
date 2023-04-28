import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import Button from 'react-bootstrap/Button';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';

export default class AddUserDialog extends Component {
    static propTypes = {
        org: PropTypes.string.isRequired,
        onSubmit: PropTypes.func.isRequired,
        onClose: PropTypes.func.isRequired,
    }

    state = {
        username: "",
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
    }


    addUser = ()=>{
        this.source.cancel();
        this.source = CancelToken.source();

        api.post("v1/CreateUser", {
            name: this.state.username,
            orgs: [this.props.org],
            roles: ["reader"],
            add_new_user: true,
        },
                 this.source.token).then(response=>{
                     if (response.cancel)
                         return;
                     this.props.onSubmit();
                 });
    }

    render() {
        return (
            <Modal show={true}
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Add a new  User")}</Modal.Title>
              </Modal.Header>
              <Modal.Body >
                <Form.Group as={Row}>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  placeholder={T("Enter a username")}
                                  rows={1}
                                  onChange={e=>this.setState({
                                          username: e.currentTarget.value})}
                                  value={this.state.username} />
                  </Col>
                </Form.Group>

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        onClick={this.addUser}>
                  {T("Do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
