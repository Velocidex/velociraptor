import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';

import {CancelToken} from 'axios';
import { PasswordChange } from './user-label.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


export default class EditUserDialog extends Component {
    static propTypes = {
        username: PropTypes.string.isRequired,
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


    render() {
        return (
            <Modal show={true}
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Update User Password")}</Modal.Title>
              </Modal.Header>
              <Modal.Body >
                <h1>
                  <div className="user-heading">
                    <FontAwesomeIcon icon="user"/>
                  </div>
                  {this.props.username}
                </h1>
                <PasswordChange
                    username={this.props.username}
                    onClose={this.props.onClose} />
              </Modal.Body>
            </Modal>
        );
    }
}
