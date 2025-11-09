import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Button from 'react-bootstrap/Button';
import UserConfig from '../core/user.jsx';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import Alert from 'react-bootstrap/Alert';
import VeloLog from "../widgets/logs.jsx";
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import "./global-messages.css";

class ShowGlobalMessages extends Component {
    static contextType = UserConfig;

    static propTypes = {
        onClose: PropTypes.func.isRequired,
    }

    alertClass = level=>{
        if (level == "ERROR") {
            return "danger";
        }
        if (level == "INFO") {
            return "info";
        }

        return "primary";
    };

    render() {
        let messages = this.context.global_messages || [];
        return (
            <Modal show={true}
                   dialogClassName="modal-70w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Global Messages")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                { _.map(messages, (x, idx)=>{
                    return <Alert variant={this.alertClass(x.level)}
                                  key={idx}>
                             <VeloLog value={x.message}/>
                           </Alert>;
                })}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={()=>this.props.onClose()}>
                  {T("Close")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


export default class GlobalMessages extends Component {
    static contextType = UserConfig;

    state = {
        show_messages: false,
    }

    buttonVariant = ()=>{
        let res = "primary";
        let msg = this.context.global_messages || [];
        for(let i=0;i<msg.length; i++) {
            if(msg[i].level == "ERROR") {
                return "danger";
            }

            if (msg[i].level == "INFO") {
                res = "info";
            }
        }

        return res;
    }

    render() {
        let msg = this.context.global_messages || [];
        return (
            <>
              <Button
                variant="primary"
                disabled={msg.length == 0}
                className={this.buttonVariant() + " global-messages"}
                onClick={()=>this.setState({show_messages: true})}
              >
                <FontAwesomeIcon icon="bell" />
              </Button>
              { this.state.show_messages &&
                <ShowGlobalMessages
                  onClose={()=>this.setState({show_messages: false})}
                /> }
            </>
        );
    }
}
