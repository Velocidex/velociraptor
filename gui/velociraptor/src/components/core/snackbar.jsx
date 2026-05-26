import _ from 'lodash';
import './snackbar.css';

import React from 'react';
import PropTypes from 'prop-types';
import Toast from 'react-bootstrap/Toast';
import ToastContainer from 'react-bootstrap/ToastContainer';

import api from '../core/api-service.jsx';

const TIMEOUT = 10 * 1000; // 10 Seconds

let guid = 1;

const getID = ()=>{
    guid++;
    return "ID" + guid;
};

// Handle errors - when the server deletes a flow or hunt or client,
// then we must clear references to them to avoid constantly querying
// for deleted resources.
const hunt_not_found = /Hunt not found/i;



export default class Snackbar extends React.Component {
    static propTypes = {
        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };


    componentDidMount = () => {
        api.hooks.push(this.warn);
    }

    addMessage = (toasts, message)=>{
        // If the same message already exists, then just show it
        // again. This prevents lots of toast spam.
        for(let i=0; i<toasts.length; i++) {
            if(message===toasts[i].body) {
                toasts[i].key = getID();
                toasts[i].show = true;
                return toasts;
            }
        }

        toasts.push({
            header: "Error",
            body: message,
            key: getID(),
        });

        // Only keep the last 5 toasts
        if(toasts.length > 5) {
            toasts = toasts.splice(toasts.length-5);
        }
        return toasts;
    }

    warn = (message) => {
        this.handle_errors(message);
        let toasts = this.addMessage(this.state.toasts || [], message);
        this.setState({toasts: toasts});
    };

    handle_errors = message=>{
        if(hunt_not_found.test(message)) {
            console.log(this.props.match);
        }
    };

    state = {
        toasts: [],
    }

    dismiss = ()=>{
        this.setState({show: false});
    }

    render() {
        return <ToastContainer>
                 {_.map(this.state.toasts, (t, idx)=>{
                     return <Toast key={t.key}
                                   show={t.show}
                                   delay={TIMEOUT}
                                   autohide
                                   bg="warning"
                                   onClose={()=>{
                         t.show = false,
                         this.setState({toasts: this.state.toasts});
                     }}>
                              <Toast.Header>
                                <strong className="me-auto">{t.header}</strong>
                              </Toast.Header>
                              <Toast.Body>{t.body}</Toast.Body>
                            </Toast>;
                 })}
               </ToastContainer>;
    }
};
